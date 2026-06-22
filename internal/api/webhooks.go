package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
)

// HandleJellyfinWebhook processes incoming events from the Jellyfin webhook plugin.
func (s *Server) HandleJellyfinWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.validateWebhookSecret(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var event jellyfin.WebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	switch event.NotificationType {
	case jellyfin.EventPlaybackStart:
		s.handlePlaybackStart(event)
	case jellyfin.EventPlaybackStop:
		s.handlePlaybackStop(event)
	case jellyfin.EventItemAdded:
		s.handleItemAdded(event)
	case jellyfin.EventItemUpdated:
		s.handleItemUpdated(event)
	case jellyfin.EventItemRemoved:
		s.handleItemRemoved(event)
	case jellyfin.EventTaskCompleted:
		s.handleTaskCompleted(event)
	case jellyfin.EventLibraryChanged:
		s.handleLibraryChanged(event)
	default:
		// Unknown events are intentionally accepted to avoid plugin retries.
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) validateWebhookSecret(r *http.Request) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	expected := strings.TrimSpace(s.cfg.Jellyfin.WebhookSecret)
	if expected == "" {
		return false
	}

	provided := strings.TrimSpace(r.Header.Get("X-Jellywatch-Webhook-Secret"))
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handlePlaybackStart(event jellyfin.WebhookEvent) {
	path := s.pathTranslator.JellyfinToDaemon(strings.TrimSpace(event.ItemPath))
	if path == "" || s.playbackLocks == nil {
		return
	}

	s.playbackLocks.Lock(path, jellyfin.PlaybackInfo{
		UserName:   event.UserName,
		DeviceName: event.DeviceName,
		ClientName: event.ClientName,
		ItemID:     event.ItemID,
		StartedAt:  time.Now(),
	})

	s.logJellyfinActivity("jellyfin_playback_start", path, event.ItemName, true, "")
}

func (s *Server) handlePlaybackStop(event jellyfin.WebhookEvent) {
	path := s.pathTranslator.JellyfinToDaemon(strings.TrimSpace(event.ItemPath))
	if path == "" {
		return
	}

	if s.playbackLocks != nil {
		s.playbackLocks.Unlock(path)
	}
	if s.deferredQueue != nil {
		_ = s.deferredQueue.RemoveForPath(path)
	}

	s.logJellyfinActivity("jellyfin_playback_stop", path, event.ItemName, true, "")
}

func identifiedFromEvent(event jellyfin.WebhookEvent) bool {
	return strings.TrimSpace(event.ProviderImdb) != "" ||
		strings.TrimSpace(event.ProviderTmdb) != "" ||
		strings.TrimSpace(event.ProviderTvdb) != ""
}

func (s *Server) handleItemAdded(event jellyfin.WebhookEvent) {
	path := s.pathTranslator.JellyfinToDaemon(strings.TrimSpace(event.ItemPath))
	itemID := strings.TrimSpace(event.ItemID)

	if s.db != nil && path != "" && itemID != "" {
		if err := s.db.UpsertJellyfinItem(path, itemID, event.ItemName, event.ItemType); err != nil {
			s.logJellyfinActivity("jellyfin_item_added", path, event.ItemName, false, err.Error())
			return
		}
		if dec, err := s.db.GetUnresolvedDecisionByTargetPath(path); err == nil && dec != nil {
			now := time.Now().UTC()
			identified := identifiedFromEvent(event)
			if updateErr := s.db.UpdateOutcome(dec.ID, database.OutcomeUpdate{
				JellyfinItemID:      itemID,
				JellyfinImdbID:      event.ProviderImdb,
				JellyfinTmdbID:      event.ProviderTmdb,
				JellyfinTvdbID:      event.ProviderTvdb,
				JellyfinResolvedAt:  &now,
				JellyfinIdentified:  &identified,
				JellyfinFirstSeenAt: &now,
			}); updateErr != nil {
				s.logJellyfinActivity("jellyfin_decision_resolve", path, event.ItemName, false, updateErr.Error())
			}
		} else if err != nil {
			s.logJellyfinActivity("jellyfin_decision_resolve", path, event.ItemName, false, err.Error())
		}
	}

	s.logJellyfinActivity("jellyfin_item_added", path, event.ItemName, true, "")
}

// handleItemUpdated is the authoritative identification signal: Jellyfin
// typically attaches metadata via Update events after the initial scan added
// a bare row. We upgrade the parse_decision in place — even if it was already
// resolved with empty ProviderIds — so identified can flip from 0 → 1.
func (s *Server) handleItemUpdated(event jellyfin.WebhookEvent) {
	path := s.pathTranslator.JellyfinToDaemon(strings.TrimSpace(event.ItemPath))
	itemID := strings.TrimSpace(event.ItemID)

	if s.db != nil && path != "" && itemID != "" {
		if err := s.db.UpsertJellyfinItem(path, itemID, event.ItemName, event.ItemType); err != nil {
			s.logJellyfinActivity("jellyfin_item_updated", path, event.ItemName, false, err.Error())
			return
		}
		if dec, err := s.db.GetDecisionByTargetPath(path); err == nil && dec != nil {
			now := time.Now().UTC()
			identified := identifiedFromEvent(event)
			if updateErr := s.db.UpgradeOutcome(dec.ID, database.OutcomeUpdate{
				JellyfinItemID:      itemID,
				JellyfinImdbID:      event.ProviderImdb,
				JellyfinTmdbID:      event.ProviderTmdb,
				JellyfinTvdbID:      event.ProviderTvdb,
				JellyfinResolvedAt:  &now,
				JellyfinIdentified:  &identified,
				JellyfinFirstSeenAt: &now,
			}); updateErr != nil {
				s.logJellyfinActivity("jellyfin_decision_upgrade", path, event.ItemName, false, updateErr.Error())
			}
		} else if err != nil {
			s.logJellyfinActivity("jellyfin_decision_upgrade", path, event.ItemName, false, err.Error())
		}
	}

	s.logJellyfinActivity("jellyfin_item_updated", path, event.ItemName, true, "")
}

// handleItemRemoved clears Jellyfin resolution on the matching parse_decision
// and flags identified=0. A removal of a file we organized is a strong signal
// something went wrong (NFO mismatch, duplicate detection, manual cleanup).
func (s *Server) handleItemRemoved(event jellyfin.WebhookEvent) {
	path := s.pathTranslator.JellyfinToDaemon(strings.TrimSpace(event.ItemPath))

	if s.db != nil && path != "" {
		if dec, err := s.db.GetDecisionByTargetPath(path); err == nil && dec != nil {
			if clearErr := s.db.ClearOutcome(dec.ID); clearErr != nil {
				s.logJellyfinActivity("jellyfin_item_removed", path, event.ItemName, false, clearErr.Error())
				return
			}
		}
	}

	s.logJellyfinActivity("jellyfin_item_removed", path, event.ItemName, false,
		"jellyfin removed item that jellywatch organized")
}

func (s *Server) handleTaskCompleted(event jellyfin.WebhookEvent) {
	s.logJellyfinActivity("jellyfin_task_completed", event.TaskName, event.ItemName, true, "")
	s.runJellyfinVerificationPass()
}

func (s *Server) handleLibraryChanged(event jellyfin.WebhookEvent) {
	s.logJellyfinActivity("jellyfin_library_changed", event.ServerName, event.ItemName, true, "")
}

func (s *Server) logJellyfinActivity(action, source, target string, success bool, errMsg string) {
	if s.activityLogger == nil {
		return
	}

	entry := activity.Entry{
		Action:    action,
		Source:    source,
		Target:    target,
		MediaType: "jellyfin",
		Success:   success,
	}
	if errMsg != "" {
		entry.Error = errMsg
	}
	_ = s.activityLogger.Log(entry)
}

func (s *Server) runJellyfinVerificationPass() {
	if s == nil || s.db == nil || s.activityLogger == nil {
		return
	}

	entries, err := s.activityLogger.GetRecentEntries(200)
	if err != nil {
		s.logJellyfinActivity("jellyfin_verification_summary", "read_activity", "", false, err.Error())
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	checked := 0
	mismatches := 0

	for _, entry := range entries {
		if entry.Action != "organize" || !entry.Success || strings.TrimSpace(entry.Target) == "" {
			continue
		}
		if !entry.Timestamp.IsZero() && entry.Timestamp.Before(cutoff) {
			continue
		}

		checked++
		item, err := s.db.GetJellyfinItemByPath(entry.Target)
		if err != nil || item == nil {
			mismatches++
			s.logJellyfinActivity("jellyfin_verification_mismatch", entry.Target, entry.ParsedTitle, false, "path not confirmed in jellyfin")
			continue
		}
	}

	s.logJellyfinActivity(
		"jellyfin_verification_summary",
		fmt.Sprintf("checked=%d", checked),
		fmt.Sprintf("mismatches=%d", mismatches),
		mismatches == 0,
		"",
	)
}
