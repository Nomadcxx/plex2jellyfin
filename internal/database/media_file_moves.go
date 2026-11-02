package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
)

// ErrMediaFileMoveInProgress reports that a media path or row is reserved by a move.
var ErrMediaFileMoveInProgress = errors.New("media file move in progress")

type mediaFileReservationQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

type mediaFileMoveReservation struct {
	fileID                int64
	replaceFileID         int64
	sourcePath            string
	targetPath            string
	canonicalPath         string
	libraryRoot           string
	targetIdentity        string
	targetQuarantinePath  string
	sourceIdentity        string
	sourceQuarantinePath  string
	replaceIdentity       string
	replaceQuarantinePath string
	clearCompliance       bool
	sourceRemovalPending  bool
	ownerPID              int
}

type mediaFileMoveReplacement struct {
	fileID         int64
	identity       string
	quarantinePath string
}

type MediaFileMoveAttempt struct {
	Published                 bool
	SourceRemoved             bool
	PublishedIdentity         string
	DestinationQuarantinePath string
	PendingSourceRemoval      <-chan error
}

func rejectReservedMediaFilePath(q mediaFileReservationQueryer, path string) error {
	var fileID int64
	err := q.QueryRow(`
		SELECT file_id FROM media_file_move_reservations
		WHERE source_path = ? OR target_path = ? LIMIT 1`, path, path).Scan(&fileID)
	if err == nil {
		return fmt.Errorf("%w: %s (move %d)", ErrMediaFileMoveInProgress, path, fileID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check media file move reservation for %s: %w", path, err)
	}
	return nil
}

func rejectReservedMediaFileID(q mediaFileReservationQueryer, fileID int64) error {
	var reservedID int64
	err := q.QueryRow(`
		SELECT file_id FROM media_file_move_reservations
		WHERE file_id = ? OR replace_file_id = ? LIMIT 1`, fileID, fileID).Scan(&reservedID)
	if err == nil {
		return fmt.Errorf("%w: media file %d", ErrMediaFileMoveInProgress, fileID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check media file move reservation for %d: %w", fileID, err)
	}
	return nil
}

// CheckMediaFileMoveDestination rejects tracked or reserved destinations before
// transfer. The reservation transaction repeats the check to close the race.
func (m *MediaDB) CheckMediaFileMoveDestination(sourcePath, destinationPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := rejectReservedMediaFilePath(m.db, sourcePath); err != nil {
		return err
	}
	if sourcePath == destinationPath {
		return nil
	}
	var targetID int64
	err := m.db.QueryRow("SELECT id FROM media_files WHERE path = ?", destinationPath).Scan(&targetID)
	if err == nil {
		return fmt.Errorf("destination path is already tracked by media file %d", targetID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read destination media file: %w", err)
	}
	return rejectReservedMediaFilePath(m.db, destinationPath)
}

// MoveMediaFileWith reserves both paths in short transactions around the
// filesystem transfer. The callback runs without a SQLite write lock.
func (m *MediaDB) MoveMediaFileWith(file *MediaFile, sourcePath, canonicalPath string, clearCompliance bool, move func(string, string, func(string, string) error) (MediaFileMoveAttempt, error)) error {
	return m.moveMediaFileWith(file, sourcePath, canonicalPath, clearCompliance, nil, move)
}

// MoveMediaFileWithReplacement replaces an existing destination while keeping
// its database row and best-file links. The callback publishes into the freed path.
func (m *MediaDB) MoveMediaFileWithReplacement(file *MediaFile, sourcePath, canonicalPath string, clearCompliance bool, move func(string, string, func(string, string) error) (MediaFileMoveAttempt, error)) error {
	if file == nil || file.Path == "" {
		return fmt.Errorf("media file replacement requires a destination")
	}
	m.mu.RLock()
	var targetID int64
	err := m.db.QueryRow("SELECT id FROM media_files WHERE path = ?", file.Path).Scan(&targetID)
	m.mu.RUnlock()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read replacement destination row: %w", err)
	}
	targetInfo, err := os.Lstat(file.Path)
	if errors.Is(err, os.ErrNotExist) {
		if targetID != 0 {
			return fmt.Errorf("replacement destination row %d has no file at %s", targetID, file.Path)
		}
		return m.MoveMediaFileWith(file, sourcePath, canonicalPath, clearCompliance, move)
	}
	if err != nil {
		return fmt.Errorf("inspect replacement destination %s: %w", file.Path, err)
	}
	contentInfo, err := os.Stat(file.Path)
	if err != nil || !contentInfo.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("destination is not a regular file")
		}
		return fmt.Errorf("inspect replacement destination %s: %w", file.Path, err)
	}
	identity, ok := fileidentity.FromInfo(targetInfo)
	if !ok {
		return fmt.Errorf("replacement destination has no stable file identity: %s", file.Path)
	}
	quarantinePath, err := fileidentity.NewQuarantinePath(file.Path)
	if err != nil {
		return err
	}
	replacement := &mediaFileMoveReplacement{fileID: targetID, identity: identity, quarantinePath: quarantinePath}
	return m.moveMediaFileWith(file, sourcePath, canonicalPath, clearCompliance, replacement, move)
}

// RetireMediaFileAfterCopy merges a retained staging row into the indexed
// destination row, preserving the destination ID and best-file links.
func (m *MediaDB) RetireMediaFileAfterCopy(sourcePath, targetPath, canonicalPath, libraryRoot string) error {
	if sourcePath == "" || targetPath == "" {
		return fmt.Errorf("retained media file reconciliation requires source and destination paths")
	}
	if filepath.Clean(sourcePath) == filepath.Clean(targetPath) {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin retained media file reconciliation: %w", err)
	}
	defer tx.Rollback()
	incoming, err := scanMediaFileRow(tx.QueryRow("SELECT "+mediaFileColumns+" FROM media_files WHERE path = ?", sourcePath))
	if err != nil {
		return fmt.Errorf("read retained source media file: %w", err)
	}
	if incoming == nil {
		return nil
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("inspect copied destination media file: %w", err)
	}
	if !targetInfo.Mode().IsRegular() {
		return fmt.Errorf("copied destination is not a regular media file: %s", targetPath)
	}
	if err := rejectReservedMediaFileID(tx, incoming.ID); err != nil {
		return err
	}
	if err := rejectReservedMediaFilePath(tx, targetPath); err != nil {
		return err
	}
	destination, err := scanMediaFileRow(tx.QueryRow("SELECT "+mediaFileColumns+" FROM media_files WHERE path = ?", targetPath))
	if err != nil {
		return fmt.Errorf("read copied destination media file: %w", err)
	}
	if destination == nil {
		result, err := tx.Exec(`UPDATE media_files SET path = ?, size = ?, modified_at = ?,
			library_root = CASE WHEN ? <> '' THEN ? ELSE library_root END,
			updated_at = CURRENT_TIMESTAMP WHERE id = ? AND path = ?`,
			targetPath, targetInfo.Size(), targetInfo.ModTime(), libraryRoot, libraryRoot, incoming.ID, sourcePath)
		if err != nil {
			return fmt.Errorf("repoint retained source media file: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check retained source path update: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("retained source media file %d was not repointed", incoming.ID)
		}
		if canonicalPath != "" {
			if err := updateMovedParent(tx,
				sql.NullInt64{Int64: valueOrZero(incoming.ParentMovieID), Valid: incoming.ParentMovieID != nil},
				sql.NullInt64{Int64: valueOrZero(incoming.ParentSeriesID), Valid: incoming.ParentSeriesID != nil},
				canonicalPath, libraryRoot); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	if destination.ID == incoming.ID {
		return nil
	}
	if err := rejectReservedMediaFileID(tx, destination.ID); err != nil {
		return err
	}
	complianceJSON, err := json.Marshal(incoming.ComplianceIssues)
	if err != nil {
		return fmt.Errorf("serialize retained source compliance metadata: %w", err)
	}
	needsReview := 0
	if incoming.NeedsReview {
		needsReview = 1
	}
	if libraryRoot == "" {
		libraryRoot = incoming.LibraryRoot
	}
	result, err := tx.Exec(`
		UPDATE media_files SET size = ?, modified_at = ?, media_type = ?, parent_movie_id = ?, parent_series_id = ?, parent_episode_id = ?,
			normalized_title = ?, year = ?, season = ?, episode = ?, resolution = ?, source_type = ?, codec = ?,
			audio_format = ?, quality_score = ?, confidence = ?, parse_method = ?, needs_review = ?,
			is_jellyfin_compliant = ?, compliance_issues = ?, source = ?, source_priority = ?, library_root = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND path = ?`,
		targetInfo.Size(), targetInfo.ModTime(), incoming.MediaType, incoming.ParentMovieID, incoming.ParentSeriesID, incoming.ParentEpisodeID,
		incoming.NormalizedTitle, incoming.Year, incoming.Season, incoming.Episode, incoming.Resolution, incoming.SourceType, incoming.Codec,
		incoming.AudioFormat, incoming.QualityScore, incoming.Confidence, incoming.ParseMethod, needsReview,
		incoming.IsJellyfinCompliant, string(complianceJSON), incoming.Source, incoming.SourcePriority, libraryRoot,
		destination.ID, targetPath)
	if err != nil {
		return fmt.Errorf("update copied destination media metadata: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check copied destination media update: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("copied destination media file %d was not updated", destination.ID)
	}
	for _, table := range []string{"movies", "episodes"} {
		if _, err := tx.Exec("UPDATE "+table+" SET best_file_id = ? WHERE best_file_id = ?", destination.ID, incoming.ID); err != nil {
			return fmt.Errorf("preserve %s best-file link after retained copy: %w", table, err)
		}
	}
	result, err = tx.Exec("DELETE FROM media_files WHERE id = ? AND path = ?", incoming.ID, sourcePath)
	if err != nil {
		return fmt.Errorf("retire retained source media file: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check retained source media retirement: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("retained source media file %d was not retired", incoming.ID)
	}
	if canonicalPath != "" {
		if err := updateMovedParent(tx,
			sql.NullInt64{Int64: valueOrZero(incoming.ParentMovieID), Valid: incoming.ParentMovieID != nil},
			sql.NullInt64{Int64: valueOrZero(incoming.ParentSeriesID), Valid: incoming.ParentSeriesID != nil},
			canonicalPath, libraryRoot); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retained media file reconciliation: %w", err)
	}
	return nil
}

func (m *MediaDB) moveMediaFileWith(file *MediaFile, sourcePath, canonicalPath string, clearCompliance bool, replacement *mediaFileMoveReplacement, move func(string, string, func(string, string) error) (MediaFileMoveAttempt, error)) error {
	if file == nil || file.ID <= 0 || sourcePath == "" || file.Path == "" || move == nil {
		return fmt.Errorf("media file move requires an existing row, source, destination, and move callback")
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		return fmt.Errorf("source has no stable file identity: %s", sourcePath)
	}
	sourceQuarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(sourceQuarantinePath)) != filepath.Clean(filepath.Dir(sourcePath)) {
		return fmt.Errorf("source quarantine path must be a sibling of %s", sourcePath)
	}

	// The reservation transaction serializes conflicting paths. Keep the file
	// transfer outside process-wide locks so unrelated moves can run together.
	if err := m.reserveMediaFileMove(file.ID, sourcePath, file.Path, canonicalPath, file.LibraryRoot, clearCompliance, sourceIdentity, sourceQuarantinePath, replacement); err != nil {
		return err
	}
	reservation := mediaFileMoveReservation{
		fileID:               file.ID,
		sourcePath:           sourcePath,
		targetPath:           file.Path,
		canonicalPath:        canonicalPath,
		libraryRoot:          file.LibraryRoot,
		sourceIdentity:       sourceIdentity,
		sourceQuarantinePath: sourceQuarantinePath,
		clearCompliance:      clearCompliance,
		ownerPID:             os.Getpid(),
	}
	if replacement != nil {
		reservation.replaceFileID = replacement.fileID
		reservation.replaceIdentity = replacement.identity
		reservation.replaceQuarantinePath = replacement.quarantinePath
		if err := fileidentity.MoveToQuarantine(file.Path, replacement.quarantinePath, replacement.identity, true); err != nil {
			if _, quarantineErr := os.Lstat(replacement.quarantinePath); errors.Is(quarantineErr, os.ErrNotExist) {
				if cancelErr := m.CancelMediaFileMove(sourcePath); cancelErr != nil {
					return errors.Join(fmt.Errorf("quarantine replacement destination: %w", err), cancelErr)
				}
			} else {
				if releaseErr := m.markMediaFileMoveRecoverable(reservation); releaseErr != nil {
					err = errors.Join(err, releaseErr)
				}
			}
			return fmt.Errorf("quarantine replacement destination: %w", err)
		}
	}
	recordPublished := func(identity, targetQuarantinePath string) error {
		if identity == "" || targetQuarantinePath == "" {
			return fmt.Errorf("published destination is missing its identity or recovery path")
		}
		if filepath.Clean(filepath.Dir(targetQuarantinePath)) != filepath.Clean(filepath.Dir(reservation.targetPath)) ||
			filepath.Clean(targetQuarantinePath) == filepath.Clean(reservation.targetPath) {
			return fmt.Errorf("destination quarantine path must be a distinct sibling of %s", reservation.targetPath)
		}
		if err := m.recordMediaFileMovePublished(reservation, identity, targetQuarantinePath); err != nil {
			return err
		}
		reservation.targetIdentity = identity
		reservation.targetQuarantinePath = targetQuarantinePath
		reservation.sourceRemovalPending = true
		return nil
	}
	attempt, moveErr := move(sourceIdentity, sourceQuarantinePath, recordPublished)
	if attempt.PublishedIdentity != "" {
		reservation.targetIdentity = attempt.PublishedIdentity
	}
	if attempt.DestinationQuarantinePath != "" {
		reservation.targetQuarantinePath = attempt.DestinationQuarantinePath
	}
	if moveErr != nil {
		if attempt.PendingSourceRemoval != nil || (attempt.Published && !attempt.SourceRemoved) {
			reservation.sourceRemovalPending = true
			if err := m.markMediaFileMovePending(reservation); err != nil {
				moveErr = errors.Join(moveErr, err)
			}
			m.finishPendingMediaFileMove(reservation, attempt.PendingSourceRemoval)
			return fmt.Errorf("move media file outcome pending reconciliation: %w", moveErr)
		}
		if attempt.Published && attempt.SourceRemoved {
			m.finishPendingMediaFileMove(reservation, nil)
			return fmt.Errorf("move media file completed at the filesystem but database work failed: %w", moveErr)
		}
		if replacement != nil {
			if reconcileErr := m.recoverMediaFileMove(reservation); reconcileErr != nil {
				if releaseErr := m.markMediaFileMoveRecoverable(reservation); releaseErr != nil {
					reconcileErr = errors.Join(reconcileErr, releaseErr)
				}
				return errors.Join(fmt.Errorf("move media file: %w", moveErr), fmt.Errorf("recover replacement move: %w", reconcileErr))
			}
			return fmt.Errorf("move media file: %w", moveErr)
		}
		if _, quarantineErr := os.Lstat(sourceQuarantinePath); quarantineErr == nil {
			if reconcileErr := m.recoverMediaFileMove(reservation); reconcileErr != nil {
				if releaseErr := m.markMediaFileMoveRecoverable(reservation); releaseErr != nil {
					reconcileErr = errors.Join(reconcileErr, releaseErr)
				}
				return errors.Join(fmt.Errorf("move media file: %w", moveErr), fmt.Errorf("recover quarantined source: %w", reconcileErr))
			}
			return fmt.Errorf("move media file: %w", moveErr)
		} else if !errors.Is(quarantineErr, os.ErrNotExist) {
			if releaseErr := m.markMediaFileMoveRecoverable(reservation); releaseErr != nil {
				quarantineErr = errors.Join(quarantineErr, releaseErr)
			}
			return errors.Join(fmt.Errorf("move media file: %w", moveErr), fmt.Errorf("inspect source quarantine: %w", quarantineErr))
		}
		if _, statErr := os.Lstat(sourcePath); statErr == nil {
			if cancelErr := m.CancelMediaFileMove(sourcePath); cancelErr != nil {
				return errors.Join(fmt.Errorf("move media file: %w", moveErr), cancelErr)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return errors.Join(fmt.Errorf("move media file: %w", moveErr), fmt.Errorf("inspect source after failed move: %w", statErr))
		}
		return fmt.Errorf("move media file: %w", moveErr)
	}
	if attempt.Published && !attempt.SourceRemoved {
		reservation.sourceRemovalPending = true
		if err := m.markMediaFileMovePending(reservation); err != nil {
			m.finishPendingMediaFileMove(reservation, nil)
			return fmt.Errorf("move completed without removing source; reconciliation pending: %w", err)
		}
		m.finishPendingMediaFileMove(reservation, nil)
		return fmt.Errorf("move completed without removing source; reconciliation pending")
	}
	if !attempt.Published || attempt.PublishedIdentity == "" {
		m.finishPendingMediaFileMove(reservation, nil)
		return fmt.Errorf("move media file completed without a verifiable published destination")
	}
	if err := verifyPublishedMediaFile(file.Path, attempt.PublishedIdentity); err != nil {
		m.finishPendingMediaFileMove(reservation, nil)
		return fmt.Errorf("verify moved media file: %w", err)
	}
	if replacement != nil {
		if err := fileidentity.RemoveQuarantine(replacement.quarantinePath, replacement.identity, true); err != nil && !errors.Is(err, os.ErrNotExist) {
			m.finishPendingMediaFileMove(reservation, nil)
			return fmt.Errorf("remove replaced destination backup: %w", err)
		}
		if err := verifyPublishedMediaFile(file.Path, attempt.PublishedIdentity); err != nil {
			m.finishPendingMediaFileMove(reservation, nil)
			return fmt.Errorf("verify replacement after destination cleanup: %w", err)
		}
		if replacement.fileID == 0 {
			if err := m.finalizeMediaFileMove(file.ID, sourcePath, file.Path, attempt.PublishedIdentity, file.Size, file.ModifiedAt); err != nil {
				m.finishPendingMediaFileMove(reservation, nil)
				return fmt.Errorf("finalize media file move after replacement: %w", err)
			}
			return nil
		}
		if err := m.finalizeReplacementMediaFileMove(reservation, sourcePath, file.Path, attempt.PublishedIdentity, file.Size, file.ModifiedAt); err != nil {
			m.finishPendingMediaFileMove(reservation, nil)
			return fmt.Errorf("finalize replacement media file move: %w", err)
		}
		return nil
	}
	if err := m.finalizeMediaFileMove(file.ID, sourcePath, file.Path, attempt.PublishedIdentity, file.Size, file.ModifiedAt); err != nil {
		m.finishPendingMediaFileMove(reservation, nil)
		return fmt.Errorf("finalize moved media file: %w", err)
	}
	return nil
}

func (m *MediaDB) markMediaFileMovePending(reservation mediaFileMoveReservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	result, err := m.db.Exec(`
		UPDATE media_file_move_reservations SET source_removal_pending = 1,
			target_identity = CASE WHEN ? <> '' THEN ? ELSE target_identity END,
			target_quarantine_path = CASE WHEN ? <> '' THEN ? ELSE target_quarantine_path END
		WHERE file_id = ? AND source_path = ? AND target_path = ? AND owner_pid = ?`,
		reservation.targetIdentity, reservation.targetIdentity,
		reservation.targetQuarantinePath, reservation.targetQuarantinePath,
		reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.ownerPID)
	if err != nil {
		return fmt.Errorf("mark source removal pending: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check pending move reservation: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("media file move reservation disappeared before pending removal")
	}
	return nil
}

func (m *MediaDB) recordMediaFileMovePublished(reservation mediaFileMoveReservation, identity, targetQuarantinePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	result, err := m.db.Exec(`
		UPDATE media_file_move_reservations SET source_removal_pending = 1, target_identity = ?, target_quarantine_path = ?
		WHERE file_id = ? AND source_path = ? AND target_path = ? AND owner_pid = ?`,
		identity, targetQuarantinePath, reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.ownerPID)
	if err != nil {
		return fmt.Errorf("record published media file identity: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check published media file reservation: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("media file move reservation disappeared before publication")
	}
	return nil
}

func (m *MediaDB) finishPendingMediaFileMove(reservation mediaFileMoveReservation, pending <-chan error) {
	go func() {
		if pending != nil {
			if err := <-pending; err != nil {
				log.Printf("plex2jellyfin: source removal for media move %s -> %s reported: %v", reservation.sourcePath, reservation.targetPath, err)
			}
		}
		var reconcileErr error
		for attempt := 0; attempt < 3; attempt++ {
			reconcileErr = m.markMediaFileMovePending(reservation)
			if reconcileErr == nil {
				reconcileErr = m.recoverMediaFileMove(reservation)
			}
			if reconcileErr == nil {
				return
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
		}
		if err := m.markMediaFileMoveRecoverable(reservation); err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
		}
		log.Printf("plex2jellyfin: media move %s -> %s remains reserved for recovery: %v", reservation.sourcePath, reservation.targetPath, reconcileErr)
	}()
}

func (m *MediaDB) markMediaFileMoveRecoverable(reservation mediaFileMoveReservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		UPDATE media_file_move_reservations SET owner_pid = 0
		WHERE file_id = ? AND source_path = ? AND target_path = ? AND owner_pid = ?`,
		reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.ownerPID)
	if err != nil {
		return fmt.Errorf("release media move for later recovery: %w", err)
	}
	return nil
}

func (m *MediaDB) reserveMediaFileMove(fileID int64, sourcePath, targetPath, canonicalPath, libraryRoot string, clearCompliance bool, sourceIdentity, sourceQuarantinePath string, replacement *mediaFileMoveReplacement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sourcePath == targetPath {
		return fmt.Errorf("media file move source and destination are identical: %s", sourcePath)
	}
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin media file move reservation: %w", err)
	}
	defer tx.Rollback()
	var currentPath string
	if err := tx.QueryRow("SELECT path FROM media_files WHERE id = ?", fileID).Scan(&currentPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("media file %d does not exist", fileID)
		}
		return fmt.Errorf("read media file %d before move: %w", fileID, err)
	}
	if currentPath != sourcePath {
		return fmt.Errorf("media file %d path changed from %s to %s", fileID, sourcePath, currentPath)
	}
	var targetID int64
	err = tx.QueryRow("SELECT id FROM media_files WHERE path = ?", targetPath).Scan(&targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read destination media file: %w", err)
	}
	if replacement == nil && err == nil {
		return fmt.Errorf("destination path is already tracked by media file %d", targetID)
	}
	if replacement != nil && targetID != replacement.fileID {
		if replacement.fileID != 0 || !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("replacement destination row changed: have %d, want %d", targetID, replacement.fileID)
		}
	}
	var reservedID int64
	err = tx.QueryRow(`
		SELECT file_id FROM media_file_move_reservations
		WHERE source_path IN (?, ?) OR target_path IN (?, ?) OR file_id = ? OR replace_file_id = ?
		   OR (? > 0 AND (file_id = ? OR replace_file_id = ?)) LIMIT 1`,
		sourcePath, targetPath, sourcePath, targetPath,
		fileID, fileID, replacementID(replacement), replacementID(replacement), replacementID(replacement)).Scan(&reservedID)
	if err == nil {
		return fmt.Errorf("%w: path already reserved by move %d", ErrMediaFileMoveInProgress, reservedID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check media file move reservations: %w", err)
	}
	clearComplianceInt := 0
	if clearCompliance {
		clearComplianceInt = 1
	}
	var replaceFileID int64
	var replaceIdentity, replaceQuarantinePath string
	if replacement != nil {
		replaceFileID = replacement.fileID
		replaceIdentity = replacement.identity
		replaceQuarantinePath = replacement.quarantinePath
	}
	if _, err := tx.Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, canonical_path, library_root, clear_compliance, owner_pid,
			 source_identity, source_quarantine_path, replace_file_id, replace_identity, replace_quarantine_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fileID, sourcePath, targetPath, canonicalPath, libraryRoot, clearComplianceInt, os.Getpid(),
		sourceIdentity, sourceQuarantinePath, replaceFileID, replaceIdentity, replaceQuarantinePath); err != nil {
		return fmt.Errorf("reserve media file move path: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media file move reservation: %w", err)
	}
	return nil
}

func replacementID(replacement *mediaFileMoveReplacement) int64 {
	if replacement == nil {
		return 0
	}
	return replacement.fileID
}

// finalizeMediaFileMove changes the media row and its parent in one short
// transaction. It reads the reservation so startup recovery uses the same data.
func (m *MediaDB) finalizeMediaFileMove(fileID int64, sourcePath, targetPath, targetIdentity string, size int64, modifiedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin media file move finalization: %w", err)
	}
	defer tx.Rollback()
	// Claim the short write transaction before reading so concurrent startup
	// recoveries serialize instead of sharing an old SQLite WAL snapshot.
	if _, err := tx.Exec(`
		UPDATE media_file_move_reservations SET owner_pid = owner_pid
		WHERE file_id = ? AND source_path = ? AND target_path = ?`, fileID, sourcePath, targetPath); err != nil {
		return fmt.Errorf("claim media file move finalization: %w", err)
	}
	reservation, err := readMediaFileMoveReservation(tx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var currentPath string
			pathErr := tx.QueryRow("SELECT path FROM media_files WHERE id = ?", fileID).Scan(&currentPath)
			if pathErr == nil && currentPath == targetPath {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("commit already-completed media file move: %w", err)
				}
				return nil
			}
		}
		return fmt.Errorf("read media file move reservation: %w", err)
	}
	if reservation.sourcePath != sourcePath || reservation.targetPath != targetPath ||
		reservation.targetIdentity == "" || reservation.targetIdentity != targetIdentity {
		return fmt.Errorf("media file move reservation no longer matches %s -> %s", sourcePath, targetPath)
	}
	var currentPath string
	var movieID, seriesID sql.NullInt64
	if err := tx.QueryRow(`
		SELECT path, parent_movie_id, parent_series_id FROM media_files WHERE id = ?`, fileID).
		Scan(&currentPath, &movieID, &seriesID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("media file %d disappeared during move", fileID)
		}
		return fmt.Errorf("read media file %d during move finalization: %w", fileID, err)
	}
	if currentPath != sourcePath {
		return fmt.Errorf("media file %d path changed during move from %s to %s", fileID, sourcePath, currentPath)
	}
	if _, err := tx.Exec("DELETE FROM media_file_move_reservations WHERE file_id = ?", fileID); err != nil {
		return fmt.Errorf("release media file move reservation: %w", err)
	}
	var result sql.Result
	if reservation.clearCompliance {
		result, err = tx.Exec(`
			UPDATE media_files SET path = ?, size = ?, modified_at = ?, library_root = ?,
				is_jellyfin_compliant = 1, compliance_issues = '[]', updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND path = ?`, targetPath, size, modifiedAt, reservation.libraryRoot, fileID, sourcePath)
	} else {
		result, err = tx.Exec(`
			UPDATE media_files SET path = ?, size = ?, modified_at = ?, library_root = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND path = ?`, targetPath, size, modifiedAt, reservation.libraryRoot, fileID, sourcePath)
	}
	if err != nil {
		return fmt.Errorf("update moved media file: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check moved media file update: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("media file %d was not updated during move", fileID)
	}
	if reservation.canonicalPath != "" {
		if err := updateMovedParent(tx, movieID, seriesID, reservation.canonicalPath, reservation.libraryRoot); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media file move finalization: %w", err)
	}
	return nil
}

func (m *MediaDB) finalizeReplacementMediaFileMove(reservation mediaFileMoveReservation, sourcePath, targetPath, targetIdentity string, size int64, modifiedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replacement media file finalization: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE media_file_move_reservations SET owner_pid = owner_pid
		WHERE file_id = ? AND source_path = ? AND target_path = ?`,
		reservation.fileID, sourcePath, targetPath); err != nil {
		return fmt.Errorf("claim replacement media file finalization: %w", err)
	}
	current, err := readMediaFileMoveReservation(tx, reservation.fileID)
	if errors.Is(err, sql.ErrNoRows) {
		var currentPath, sourceCurrentPath string
		destinationErr := tx.QueryRow("SELECT path FROM media_files WHERE id = ?", reservation.replaceFileID).Scan(&currentPath)
		sourceErr := tx.QueryRow("SELECT path FROM media_files WHERE id = ?", reservation.fileID).Scan(&sourceCurrentPath)
		if destinationErr == nil && currentPath == targetPath && errors.Is(sourceErr, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit already-completed replacement move: %w", err)
			}
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("read replacement move reservation: %w", err)
	}
	if current.replaceFileID != reservation.replaceFileID || current.replaceFileID <= 0 ||
		current.sourcePath != sourcePath || current.targetPath != targetPath ||
		current.targetIdentity != targetIdentity || current.replaceIdentity == "" {
		return fmt.Errorf("replacement media file move reservation no longer matches %s -> %s", sourcePath, targetPath)
	}
	source, err := scanMediaFileRow(tx.QueryRow("SELECT "+mediaFileColumns+" FROM media_files WHERE id = ?", current.fileID))
	if err != nil {
		return fmt.Errorf("read incoming media file during replacement: %w", err)
	}
	if source == nil || source.Path != sourcePath {
		return fmt.Errorf("incoming media file %d no longer points to %s", current.fileID, sourcePath)
	}
	var destinationPath string
	if err := tx.QueryRow("SELECT path FROM media_files WHERE id = ?", current.replaceFileID).Scan(&destinationPath); err != nil {
		return fmt.Errorf("read replacement destination media file %d: %w", current.replaceFileID, err)
	}
	if destinationPath != targetPath {
		return fmt.Errorf("replacement destination media file %d path changed to %s", current.replaceFileID, destinationPath)
	}
	if _, err := tx.Exec(`DELETE FROM media_file_move_reservations WHERE file_id = ? AND source_path = ? AND target_path = ?`, current.fileID, sourcePath, targetPath); err != nil {
		return fmt.Errorf("release replacement media file reservation: %w", err)
	}
	complianceJSON, err := json.Marshal(source.ComplianceIssues)
	if err != nil {
		return fmt.Errorf("serialize incoming compliance metadata: %w", err)
	}
	compliant := source.IsJellyfinCompliant
	if current.clearCompliance {
		compliant = true
		complianceJSON = []byte("[]")
	}
	needsReview := 0
	if source.NeedsReview {
		needsReview = 1
	}
	result, err := tx.Exec(`
		UPDATE media_files SET
			size = ?, modified_at = ?, media_type = ?, parent_movie_id = ?, parent_series_id = ?, parent_episode_id = ?,
			normalized_title = ?, year = ?, season = ?, episode = ?, resolution = ?, source_type = ?, codec = ?,
			audio_format = ?, quality_score = ?, confidence = ?, parse_method = ?, needs_review = ?,
			is_jellyfin_compliant = ?, compliance_issues = ?, source = ?, source_priority = ?, library_root = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND path = ?`,
		size, modifiedAt, source.MediaType, source.ParentMovieID, source.ParentSeriesID, source.ParentEpisodeID,
		source.NormalizedTitle, source.Year, source.Season, source.Episode, source.Resolution, source.SourceType, source.Codec,
		source.AudioFormat, source.QualityScore, source.Confidence, source.ParseMethod, needsReview,
		compliant, string(complianceJSON), source.Source, source.SourcePriority, current.libraryRoot,
		current.replaceFileID, targetPath)
	if err != nil {
		return fmt.Errorf("update replacement destination metadata: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check replacement destination metadata update: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("replacement destination media file %d was not updated", current.replaceFileID)
	}
	for _, table := range []string{"movies", "episodes"} {
		if _, err := tx.Exec("UPDATE "+table+" SET best_file_id = ? WHERE best_file_id = ?", current.replaceFileID, current.fileID); err != nil {
			return fmt.Errorf("preserve %s best-file link during replacement: %w", table, err)
		}
	}
	result, err = tx.Exec("DELETE FROM media_files WHERE id = ? AND path = ?", current.fileID, sourcePath)
	if err != nil {
		return fmt.Errorf("remove incoming media file row after replacement: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check incoming media file row removal: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("incoming media file %d was not removed after replacement", current.fileID)
	}
	if current.canonicalPath != "" {
		if err := updateMovedParent(tx, sql.NullInt64{Int64: valueOrZero(source.ParentMovieID), Valid: source.ParentMovieID != nil},
			sql.NullInt64{Int64: valueOrZero(source.ParentSeriesID), Valid: source.ParentSeriesID != nil}, current.canonicalPath, current.libraryRoot); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replacement media file finalization: %w", err)
	}
	return nil
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func readMediaFileMoveReservation(q mediaFileReservationQueryer, fileID int64) (mediaFileMoveReservation, error) {
	var reservation mediaFileMoveReservation
	var clearComplianceInt, sourceRemovalPendingInt int
	err := q.QueryRow(`
		SELECT file_id, source_path, target_path, canonical_path, library_root, target_identity, target_quarantine_path,
			source_identity, source_quarantine_path, replace_file_id, replace_identity, replace_quarantine_path,
			clear_compliance, source_removal_pending, owner_pid
		FROM media_file_move_reservations WHERE file_id = ?`, fileID).Scan(
		&reservation.fileID, &reservation.sourcePath, &reservation.targetPath,
		&reservation.canonicalPath, &reservation.libraryRoot, &reservation.targetIdentity, &reservation.targetQuarantinePath,
		&reservation.sourceIdentity, &reservation.sourceQuarantinePath, &reservation.replaceFileID,
		&reservation.replaceIdentity, &reservation.replaceQuarantinePath, &clearComplianceInt,
		&sourceRemovalPendingInt, &reservation.ownerPID)
	if err != nil {
		return mediaFileMoveReservation{}, err
	}
	reservation.clearCompliance = clearComplianceInt != 0
	reservation.sourceRemovalPending = sourceRemovalPendingInt != 0
	return reservation, nil
}

func updateMovedParent(tx *sql.Tx, movieID, seriesID sql.NullInt64, canonicalPath, libraryRoot string) error {
	if movieID.Valid && seriesID.Valid {
		return fmt.Errorf("media file has both movie and series parents")
	}
	var query string
	var parentID int64
	switch {
	case movieID.Valid:
		query = `UPDATE movies SET canonical_path = ?, library_root = ?, updated_at = CURRENT_TIMESTAMP, radarr_path_dirty = 1 WHERE id = ?`
		parentID = movieID.Int64
	case seriesID.Valid:
		query = `UPDATE series SET canonical_path = ?, library_root = ?, updated_at = CURRENT_TIMESTAMP, sonarr_path_dirty = 1 WHERE id = ?`
		parentID = seriesID.Int64
	default:
		return nil
	}
	result, err := tx.Exec(query, canonicalPath, libraryRoot, parentID)
	if err != nil {
		return fmt.Errorf("update moved media parent path: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check moved media parent update: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("media parent %d does not exist", parentID)
	}
	return nil
}

// CancelMediaFileMove releases a reservation after the caller has restored the
// source file. It must not be called while a completed move remains at target.
func (m *MediaDB) CancelMediaFileMove(sourcePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.db.Exec("DELETE FROM media_file_move_reservations WHERE source_path = ?", sourcePath); err != nil {
		return fmt.Errorf("cancel media file move reservation for %s: %w", sourcePath, err)
	}
	return nil
}

// RecoverStaleMediaFileMoves reconciles reservations left by dead processes.
func (m *MediaDB) RecoverStaleMediaFileMoves() error {
	m.mu.RLock()
	rows, err := m.db.Query(`
		SELECT file_id, source_path, target_path, canonical_path, library_root, target_identity, target_quarantine_path,
			source_identity, source_quarantine_path, replace_file_id, replace_identity, replace_quarantine_path,
			clear_compliance, source_removal_pending, owner_pid
		FROM media_file_move_reservations ORDER BY created_at, file_id`)
	if err != nil {
		m.mu.RUnlock()
		return fmt.Errorf("list media file move reservations: %w", err)
	}
	var reservations []mediaFileMoveReservation
	for rows.Next() {
		var reservation mediaFileMoveReservation
		var clearComplianceInt, sourceRemovalPendingInt int
		if err := rows.Scan(
			&reservation.fileID, &reservation.sourcePath, &reservation.targetPath,
			&reservation.canonicalPath, &reservation.libraryRoot, &reservation.targetIdentity, &reservation.targetQuarantinePath,
			&reservation.sourceIdentity, &reservation.sourceQuarantinePath, &reservation.replaceFileID,
			&reservation.replaceIdentity, &reservation.replaceQuarantinePath, &clearComplianceInt,
			&sourceRemovalPendingInt, &reservation.ownerPID); err != nil {
			rows.Close()
			m.mu.RUnlock()
			return fmt.Errorf("read media file move reservation: %w", err)
		}
		reservation.clearCompliance = clearComplianceInt != 0
		reservation.sourceRemovalPending = sourceRemovalPendingInt != 0
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		m.mu.RUnlock()
		return fmt.Errorf("read media file move reservations: %w", err)
	}
	if err := rows.Close(); err != nil {
		m.mu.RUnlock()
		return fmt.Errorf("close media file move reservations: %w", err)
	}
	m.mu.RUnlock()

	for _, reservation := range reservations {
		if processIsRunning(reservation.ownerPID) {
			continue
		}
		claimed, err := m.claimStaleMediaFileMove(&reservation)
		if err != nil {
			return fmt.Errorf("claim stale media file move %d: %w", reservation.fileID, err)
		}
		if !claimed {
			continue
		}
		if err := m.recoverMediaFileMove(reservation); err != nil {
			return fmt.Errorf("recover media file move %d: %w", reservation.fileID, err)
		}
	}
	return nil
}

func (m *MediaDB) claimStaleMediaFileMove(reservation *mediaFileMoveReservation) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result, err := m.db.Exec(`
		UPDATE media_file_move_reservations SET owner_pid = ?
		WHERE file_id = ? AND owner_pid = ? AND source_path = ? AND target_path = ?`,
		os.Getpid(), reservation.fileID, reservation.ownerPID, reservation.sourcePath, reservation.targetPath)
	if err != nil {
		return false, fmt.Errorf("update reservation owner: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check reservation ownership: %w", err)
	}
	if rowsAffected != 1 {
		return false, nil
	}
	reservation.ownerPID = os.Getpid()
	return true, nil
}

func (m *MediaDB) recoverReplacementMediaFileMove(reservation mediaFileMoveReservation) error {
	for _, path := range []string{reservation.sourceQuarantinePath, reservation.targetQuarantinePath, reservation.replaceQuarantinePath} {
		if path != "" {
			parent := reservation.sourcePath
			if path != reservation.sourceQuarantinePath {
				parent = reservation.targetPath
			}
			if filepath.Clean(filepath.Dir(path)) != filepath.Clean(filepath.Dir(parent)) {
				return fmt.Errorf("replacement quarantine is not beside its media path: %s", path)
			}
		}
	}
	if reservation.replaceIdentity == "" || reservation.replaceQuarantinePath == "" {
		return fmt.Errorf("replacement move is missing its destination identity or quarantine path")
	}

	// Restore a published file hidden during a crash between publication and
	// the transfer callback's completion.
	if reservation.targetQuarantinePath != "" {
		publishedQuarantine, err := os.Lstat(reservation.targetQuarantinePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect published destination quarantine: %w", err)
		}
		if err == nil {
			quarantineIdentity, ok := fileidentity.FromInfo(publishedQuarantine)
			if !ok {
				return fmt.Errorf("published destination quarantine has no stable identity")
			}
			_, targetErr := os.Lstat(reservation.targetPath)
			if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
				return fmt.Errorf("inspect destination during replacement recovery: %w", targetErr)
			}
			if targetErr == nil {
				if reservation.targetIdentity == "" || quarantineIdentity != reservation.targetIdentity ||
					!fileidentity.MatchesLstat(reservation.targetPath, reservation.targetIdentity) {
					return fmt.Errorf("destination and published quarantine both exist with different identities; leaving both in place")
				}
				if err := fileidentity.RemoveQuarantine(reservation.targetQuarantinePath, reservation.targetIdentity, true); err != nil {
					return fmt.Errorf("remove duplicate published quarantine: %w", err)
				}
			} else if err := fileidentity.RestoreQuarantine(reservation.targetQuarantinePath, reservation.targetPath, quarantineIdentity, true); err != nil {
				return fmt.Errorf("restore published destination during replacement recovery: %w", err)
			}
		}
	}

	_, sourceErr := os.Lstat(reservation.sourcePath)
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return fmt.Errorf("inspect replacement source: %w", sourceErr)
	}
	sourceExists := sourceErr == nil
	sourceMatches := sourceExists && reservation.sourceIdentity != "" && fileidentity.MatchesLstat(reservation.sourcePath, reservation.sourceIdentity)
	sourceQuarantineInfo, sourceQuarantineErr := os.Lstat(reservation.sourceQuarantinePath)
	if reservation.sourceQuarantinePath == "" {
		sourceQuarantineErr = os.ErrNotExist
	}
	if sourceQuarantineErr != nil && !errors.Is(sourceQuarantineErr, os.ErrNotExist) {
		return fmt.Errorf("inspect replacement source quarantine: %w", sourceQuarantineErr)
	}
	sourceQuarantineExists := sourceQuarantineErr == nil
	targetInfo, targetErr := os.Lstat(reservation.targetPath)
	if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return fmt.Errorf("inspect replacement destination: %w", targetErr)
	}
	targetExists := targetErr == nil
	targetMatches := targetExists && reservation.targetIdentity != "" &&
		fileidentity.MatchesLstat(reservation.targetPath, reservation.targetIdentity)
	published := false
	if targetMatches {
		contentInfo, err := os.Stat(reservation.targetPath)
		if err != nil {
			return fmt.Errorf("inspect published replacement destination: %w", err)
		}
		published = contentInfo.Mode().IsRegular()
	}

	_, backupErr := os.Lstat(reservation.replaceQuarantinePath)
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return fmt.Errorf("inspect old destination quarantine: %w", backupErr)
	}
	backupExists := backupErr == nil
	backupMatches := backupExists && fileidentity.MatchesLstat(reservation.replaceQuarantinePath, reservation.replaceIdentity)
	if backupExists && !backupMatches {
		return fmt.Errorf("old destination quarantine identity changed; leaving it in place")
	}

	if published {
		if sourceExists && sourceMatches {
			if !backupExists {
				return fmt.Errorf("source remains after replacement but the old destination backup is missing; leaving both files in place")
			}
			// The move did not remove its original source. Roll back the published
			// copy and put the old canonical file back before releasing the rows.
			if err := fileidentity.RemoveIfMatches(reservation.targetPath, reservation.targetIdentity, true); err != nil {
				return fmt.Errorf("remove incomplete published replacement: %w", err)
			}
			if backupExists {
				if err := fileidentity.RestoreQuarantine(reservation.replaceQuarantinePath, reservation.targetPath, reservation.replaceIdentity, true); err != nil {
					return fmt.Errorf("restore old destination after incomplete replacement: %w", err)
				}
				backupExists = false
			}
			if sourceQuarantineExists {
				if err := fileidentity.RemoveQuarantine(reservation.sourceQuarantinePath, reservation.sourceIdentity, true); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove duplicate source quarantine after replacement rollback: %w", err)
				}
			}
			return m.cancelStaleMediaFileMove(reservation)
		}
		if sourceQuarantineExists {
			quarantineIdentity, ok := fileidentity.FromInfo(sourceQuarantineInfo)
			if !ok {
				return fmt.Errorf("replacement source quarantine has no stable identity")
			}
			if sourceIdentity := reservation.sourceIdentity; quarantineIdentity == sourceIdentity {
				if err := fileidentity.RemoveQuarantine(reservation.sourceQuarantinePath, sourceIdentity, true); err != nil {
					return fmt.Errorf("finish source removal after replacement: %w", err)
				}
			} else {
				if sourceExists {
					return fmt.Errorf("replacement source and its quarantine both exist; leaving both in place")
				}
				if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, quarantineIdentity, true); err != nil {
					return fmt.Errorf("restore changed source after replacement: %w", err)
				}
			}
		}
		if backupExists {
			if err := fileidentity.RemoveQuarantine(reservation.replaceQuarantinePath, reservation.replaceIdentity, true); err != nil {
				return fmt.Errorf("remove replaced destination backup: %w", err)
			}
		}
		if err := verifyPublishedMediaFile(reservation.targetPath, reservation.targetIdentity); err != nil {
			return fmt.Errorf("verify replacement before database recovery: %w", err)
		}
		if reservation.replaceFileID > 0 {
			return m.finalizeReplacementMediaFileMove(reservation, reservation.sourcePath, reservation.targetPath,
				reservation.targetIdentity, targetInfo.Size(), targetInfo.ModTime())
		}
		return m.finalizeMediaFileMove(reservation.fileID, reservation.sourcePath, reservation.targetPath,
			reservation.targetIdentity, targetInfo.Size(), targetInfo.ModTime())
	}

	if targetExists {
		if !fileidentity.MatchesLstat(reservation.targetPath, reservation.replaceIdentity) {
			return fmt.Errorf("replacement destination identity changed; leaving it in place")
		}
		if backupExists {
			if err := fileidentity.RemoveQuarantine(reservation.replaceQuarantinePath, reservation.replaceIdentity, true); err != nil {
				return fmt.Errorf("remove duplicate old destination quarantine: %w", err)
			}
			backupExists = false
		}
	} else if backupExists {
		if err := fileidentity.RestoreQuarantine(reservation.replaceQuarantinePath, reservation.targetPath, reservation.replaceIdentity, true); err != nil {
			return fmt.Errorf("restore old destination after failed replacement: %w", err)
		}
	} else {
		return fmt.Errorf("old destination and published replacement are both missing for %s", reservation.targetPath)
	}
	if sourceQuarantineExists {
		quarantineIdentity, ok := fileidentity.FromInfo(sourceQuarantineInfo)
		if !ok {
			return fmt.Errorf("replacement source quarantine has no stable identity")
		}
		if sourceExists {
			return fmt.Errorf("replacement source and its quarantine both exist; leaving both in place")
		}
		if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, quarantineIdentity, true); err != nil {
			return fmt.Errorf("restore source after failed replacement: %w", err)
		}
		sourceExists = true
	}
	if !sourceExists {
		return fmt.Errorf("replacement source is missing after restoring destination %s", reservation.targetPath)
	}
	return m.cancelStaleMediaFileMove(reservation)
}

func (m *MediaDB) recoverMediaFileMove(reservation mediaFileMoveReservation) error {
	if reservation.replaceIdentity != "" {
		return m.recoverReplacementMediaFileMove(reservation)
	}
	if reservation.replaceFileID > 0 || reservation.replaceQuarantinePath != "" {
		return fmt.Errorf("replacement reservation is missing its destination identity")
	}
	if reservation.sourceQuarantinePath != "" && filepath.Clean(filepath.Dir(reservation.sourceQuarantinePath)) != filepath.Clean(filepath.Dir(reservation.sourcePath)) {
		return fmt.Errorf("source quarantine is not beside %s", reservation.sourcePath)
	}
	if reservation.targetQuarantinePath != "" && filepath.Clean(filepath.Dir(reservation.targetQuarantinePath)) != filepath.Clean(filepath.Dir(reservation.targetPath)) {
		return fmt.Errorf("destination quarantine is not beside %s", reservation.targetPath)
	}

	_, sourceErr := os.Lstat(reservation.sourcePath)
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return fmt.Errorf("inspect source %s: %w", reservation.sourcePath, sourceErr)
	}
	_, sourceQuarantineErr := os.Lstat(reservation.sourceQuarantinePath)
	if reservation.sourceQuarantinePath == "" {
		sourceQuarantineErr = os.ErrNotExist
	}
	if sourceQuarantineErr != nil && !errors.Is(sourceQuarantineErr, os.ErrNotExist) {
		return fmt.Errorf("inspect source quarantine %s: %w", reservation.sourceQuarantinePath, sourceQuarantineErr)
	}
	targetInfo, targetErr := os.Lstat(reservation.targetPath)
	if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return fmt.Errorf("inspect destination entry %s: %w", reservation.targetPath, targetErr)
	}
	_, targetQuarantineErr := os.Lstat(reservation.targetQuarantinePath)
	if reservation.targetQuarantinePath == "" {
		targetQuarantineErr = os.ErrNotExist
	}
	if targetQuarantineErr != nil && !errors.Is(targetQuarantineErr, os.ErrNotExist) {
		return fmt.Errorf("inspect destination quarantine %s: %w", reservation.targetQuarantinePath, targetQuarantineErr)
	}

	sourceExists := sourceErr == nil
	sourceQuarantineExists := sourceQuarantineErr == nil
	targetExists := targetErr == nil
	targetQuarantineExists := targetQuarantineErr == nil
	targetMatches := targetExists && reservation.targetIdentity != "" &&
		fileidentity.MatchesLstat(reservation.targetPath, reservation.targetIdentity)
	targetQuarantineMatches := targetQuarantineExists && reservation.targetIdentity != "" &&
		fileidentity.MatchesLstat(reservation.targetQuarantinePath, reservation.targetIdentity)

	// A crash can leave the published file hidden after both paths were
	// quarantined. Restore that exact file before deciding whether to finish or
	// roll back the source move.
	if targetQuarantineExists {
		if !targetQuarantineMatches {
			quarantineIdentity, ok := fileidentity.FromLstat(reservation.targetQuarantinePath)
			if !ok {
				return fmt.Errorf("destination quarantine identity cannot be read for move %s -> %s; leaving it in place", reservation.sourcePath, reservation.targetPath)
			}
			if targetExists {
				return fmt.Errorf("destination and mismatched quarantine both exist for move %s -> %s; leaving both in place", reservation.sourcePath, reservation.targetPath)
			}
			if err := fileidentity.RestoreQuarantine(reservation.targetQuarantinePath, reservation.targetPath, quarantineIdentity, true); err != nil {
				return fmt.Errorf("restore changed destination after interrupted move %s -> %s: %w", reservation.sourcePath, reservation.targetPath, err)
			}
			targetInfo, targetErr = os.Lstat(reservation.targetPath)
			if targetErr != nil {
				return fmt.Errorf("inspect restored destination %s: %w", reservation.targetPath, targetErr)
			}
			targetExists = true
			targetMatches = false
			targetQuarantineExists = false
			targetQuarantineMatches = false
		} else if targetExists {
			if !targetMatches {
				if sourceExists && !sourceQuarantineExists && reservation.sourceIdentity != "" &&
					fileidentity.MatchesLstat(reservation.sourcePath, reservation.sourceIdentity) {
					if err := fileidentity.RemoveQuarantine(reservation.targetQuarantinePath, reservation.targetIdentity, true); err != nil {
						return fmt.Errorf("remove published copy after confirming source remains: %w", err)
					}
					return m.cancelStaleMediaFileMove(reservation)
				}
				if !sourceExists {
					if sourceQuarantineExists {
						sourceQuarantineIdentity, ok := fileidentity.FromLstat(reservation.sourceQuarantinePath)
						if !ok {
							return fmt.Errorf("source quarantine identity cannot be read for move %s -> %s; leaving it in place", reservation.sourcePath, reservation.targetPath)
						}
						if sourceQuarantineIdentity == reservation.sourceIdentity {
							if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, reservation.sourceIdentity, true); err != nil {
								return fmt.Errorf("restore source after destination replacement: %w", err)
							}
							if err := fileidentity.RemoveQuarantine(reservation.targetQuarantinePath, reservation.targetIdentity, true); err != nil {
								return fmt.Errorf("remove published copy after restoring source: %w", err)
							}
							return m.cancelStaleMediaFileMove(reservation)
						}
						if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, sourceQuarantineIdentity, true); err != nil {
							return fmt.Errorf("restore changed source after destination replacement: %w", err)
						}
						return fmt.Errorf("destination path is occupied during recovery of move %s -> %s; source replacement restored and published file retained at %s", reservation.sourcePath, reservation.targetPath, reservation.targetQuarantinePath)
					}
					if err := fileidentity.RestoreQuarantine(reservation.targetQuarantinePath, reservation.sourcePath, reservation.targetIdentity, true); err != nil {
						return fmt.Errorf("restore published file to source after destination replacement: %w", err)
					}
					return m.cancelStaleMediaFileMove(reservation)
				}
				return fmt.Errorf("destination path is occupied during recovery of move %s -> %s; leaving both files in place", reservation.sourcePath, reservation.targetPath)
			}
			if err := fileidentity.RemoveQuarantine(reservation.targetQuarantinePath, reservation.targetIdentity, true); err != nil {
				return fmt.Errorf("remove duplicate destination quarantine for %s: %w", reservation.targetPath, err)
			}
		} else if err := fileidentity.RestoreQuarantine(reservation.targetQuarantinePath, reservation.targetPath, reservation.targetIdentity, true); err != nil {
			return fmt.Errorf("restore published destination for move %s -> %s: %w", reservation.sourcePath, reservation.targetPath, err)
		}
		if targetQuarantineMatches {
			targetInfo, targetErr = os.Lstat(reservation.targetPath)
			if targetErr != nil {
				return fmt.Errorf("inspect restored destination %s: %w", reservation.targetPath, targetErr)
			}
			targetExists = true
			targetMatches = fileidentity.MatchesLstat(reservation.targetPath, reservation.targetIdentity)
		}
	}

	var publishedInfo os.FileInfo
	if targetExists && targetMatches {
		publishedInfo, targetErr = os.Stat(reservation.targetPath)
		if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
			return fmt.Errorf("inspect destination %s: %w", reservation.targetPath, targetErr)
		}
	}
	published := targetMatches && targetErr == nil && publishedInfo != nil && publishedInfo.Mode().IsRegular()

	if sourceQuarantineExists {
		quarantinedSourceIdentity, ok := fileidentity.FromLstat(reservation.sourceQuarantinePath)
		if !ok {
			return fmt.Errorf("source quarantine identity cannot be read for move %s -> %s; leaving it in place", reservation.sourcePath, reservation.targetPath)
		}
		if reservation.sourceIdentity == "" || quarantinedSourceIdentity != reservation.sourceIdentity {
			// The crash happened after a replacement entered quarantine but before
			// its identity check restored it. Put the captured object back first.
			if sourceExists {
				return fmt.Errorf("source and source quarantine both exist for move %s -> %s; leaving them in place", reservation.sourcePath, reservation.targetPath)
			}
			if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, quarantinedSourceIdentity, true); err != nil {
				return fmt.Errorf("restore changed source after interrupted move %s -> %s: %w", reservation.sourcePath, reservation.targetPath, err)
			}
			if published {
				return m.finalizeMediaFileMove(reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.targetIdentity, publishedInfo.Size(), publishedInfo.ModTime())
			}
			if targetExists && targetMatches {
				if err := fileidentity.RemoveIfMatches(reservation.targetPath, reservation.targetIdentity, true); err != nil {
					return fmt.Errorf("remove unusable published destination %s: %w", reservation.targetPath, err)
				}
			}
			return m.cancelStaleMediaFileMove(reservation)
		}

		if published {
			if err := fileidentity.RemoveQuarantine(reservation.sourceQuarantinePath, reservation.sourceIdentity, true); err != nil {
				return fmt.Errorf("finish source removal for move %s -> %s: %w", reservation.sourcePath, reservation.targetPath, err)
			}
			return m.finalizeMediaFileMove(reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.targetIdentity, publishedInfo.Size(), publishedInfo.ModTime())
		}
		if sourceExists {
			return fmt.Errorf("cannot restore source quarantine for move %s -> %s because the source path is occupied", reservation.sourcePath, reservation.targetPath)
		}
		if err := fileidentity.RestoreQuarantine(reservation.sourceQuarantinePath, reservation.sourcePath, reservation.sourceIdentity, true); err != nil {
			return fmt.Errorf("restore source after incomplete move %s -> %s: %w", reservation.sourcePath, reservation.targetPath, err)
		}
		if targetExists && targetMatches {
			if err := fileidentity.RemoveIfMatches(reservation.targetPath, reservation.targetIdentity, true); err != nil {
				return fmt.Errorf("remove unusable published destination %s: %w", reservation.targetPath, err)
			}
		}
		return m.cancelStaleMediaFileMove(reservation)
	}

	if sourceExists {
		sourceMatches := reservation.sourceIdentity == "" || fileidentity.MatchesLstat(reservation.sourcePath, reservation.sourceIdentity)
		if published {
			if !sourceMatches {
				// The path was replaced while the transfer ran. Keep the new source
				// and let the tracked row follow its verified published copy.
				return m.finalizeMediaFileMove(reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.targetIdentity, publishedInfo.Size(), publishedInfo.ModTime())
			}
			if !reservation.sourceRemovalPending {
				return fmt.Errorf("cannot reconcile move %s -> %s because source removal was not recorded", reservation.sourcePath, reservation.targetPath)
			}
			if err := fileidentity.RemoveIfMatches(reservation.targetPath, reservation.targetIdentity, true); err != nil {
				return fmt.Errorf("remove published destination while source remains: %w", err)
			}
			return m.cancelStaleMediaFileMove(reservation)
		}
		if targetExists && targetMatches && targetInfo.Mode()&os.ModeSymlink != 0 &&
			errors.Is(targetErr, os.ErrNotExist) && reservation.sourceRemovalPending && sourceMatches {
			if err := fileidentity.RemoveIfMatches(reservation.targetPath, reservation.targetIdentity, true); err != nil {
				return fmt.Errorf("remove broken published destination while source remains: %w", err)
			}
			return m.cancelStaleMediaFileMove(reservation)
		}
		if targetExists && reservation.targetIdentity != "" && !targetMatches && reservation.sourceRemovalPending {
			return fmt.Errorf("destination identity changed during move %s -> %s; leaving it in place", reservation.sourcePath, reservation.targetPath)
		}
		return m.cancelStaleMediaFileMove(reservation)
	}

	if published {
		return m.finalizeMediaFileMove(reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.targetIdentity, publishedInfo.Size(), publishedInfo.ModTime())
	}
	if !targetExists {
		return fmt.Errorf("both source and destination are missing for move %s -> %s", reservation.sourcePath, reservation.targetPath)
	}
	return fmt.Errorf("destination identity cannot be verified for move %s -> %s; leaving it in place", reservation.sourcePath, reservation.targetPath)
}

func verifyPublishedMediaFile(path, identity string) error {
	if identity == "" {
		return fmt.Errorf("published destination has no stable file identity")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect published destination %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || !fileidentity.MatchesLstat(path, identity) {
		return fmt.Errorf("published destination identity changed: %s", path)
	}
	return nil
}

func (m *MediaDB) cancelStaleMediaFileMove(reservation mediaFileMoveReservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.db.Exec(`
		DELETE FROM media_file_move_reservations
		WHERE file_id = ? AND owner_pid = ?`, reservation.fileID, reservation.ownerPID); err != nil {
		return fmt.Errorf("release stale reservation: %w", err)
	}
	return nil
}
