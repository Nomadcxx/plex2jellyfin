package housekeeping

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

// taskProgress is a thin adapter that pushes housekeeping task lifecycle
// frames into the IPC OpRegistry so the WebUI can subscribe via SSE at
// /api/v1/events/op/hk-task-<id>.
//
// All methods are nil-safe: when no registry is wired (e.g., in unit
// tests), every call is a no-op.
type taskProgress struct {
	op     *ipc.Op
	reg    *ipc.OpRegistry
	id     string
	totalB int64
	totalN int
	src    string
	dst    string
	final  bool
}

const taskOpCmd ipc.Command = "HK_TASK"

// startTaskOp registers a new op with the registry and emits an initial
// progress frame describing the work envelope.
func (e *Engine) startTaskOp(taskID int64, src, dst string, totalFiles int, totalBytes int64) *taskProgress {
	tp := &taskProgress{
		id:     fmt.Sprintf("hk-task-%d", taskID),
		totalB: totalBytes,
		totalN: totalFiles,
		src:    src,
		dst:    dst,
	}
	if e.registry == nil {
		return tp
	}
	op, err := e.registry.Start(tp.id, taskOpCmd, func() {})
	if err != nil {
		return tp
	}
	tp.op = op
	tp.reg = e.registry
	op.Frames.Append(ipc.Frame{
		ID:      tp.id,
		Type:    ipc.FrameProgress,
		Phase:   "claimed",
		Msg:     fmt.Sprintf("merge %s → %s", filepath.Base(src), filepath.Base(dst)),
		Current: 0,
		Total:   int(totalBytes / (1 << 20)),
	})
	return tp
}

func (p *taskProgress) frame(phase, msg string, currentBytes, totalBytes int64) {
	if p == nil || p.op == nil {
		return
	}
	p.op.Frames.Append(ipc.Frame{
		ID:      p.id,
		Type:    ipc.FrameProgress,
		Phase:   phase,
		Msg:     msg,
		Current: int(currentBytes / (1 << 20)),
		Total:   int(totalBytes / (1 << 20)),
	})
}

func (p *taskProgress) fileStarted(rel string, size, doneBytes, totalBytes int64) {
	p.frame("file_started", fmt.Sprintf("%s (%d MB)", rel, size/(1<<20)), doneBytes, totalBytes)
}

func (p *taskProgress) fileBytes(rel string, fileCur, fileSize, doneBytes, totalBytes int64) {
	p.frame("transferring",
		fmt.Sprintf("%s %d/%d MB", rel, fileCur/(1<<20), fileSize/(1<<20)),
		doneBytes, totalBytes)
}

func (p *taskProgress) fileCompleted(rel string, size, doneBytes, totalBytes int64) {
	p.frame("file_done", fmt.Sprintf("%s ✓", rel), doneBytes, totalBytes)
}

func (p *taskProgress) fileSkipped(rel string, size, doneBytes, totalBytes int64) {
	p.frame("file_skipped", fmt.Sprintf("%s (dup)", rel), doneBytes, totalBytes)
}

func (p *taskProgress) fileFailed(rel string, err error) {
	p.frame("file_error", fmt.Sprintf("%s: %v", rel, err), 0, p.totalB)
}

func (p *taskProgress) summary(moved, skipped int, doneBytes, totalBytes int64) {
	p.frame("summary",
		fmt.Sprintf("moved=%d skipped=%d bytes=%d", moved, skipped, doneBytes),
		doneBytes, totalBytes)
}

// finish closes out the op. Pass a non-nil err to mark it failed, nil for
// success. Safe to call multiple times.
func (p *taskProgress) finish(err error) {
	if p == nil || p.op == nil || p.final {
		return
	}
	p.final = true
	if err != nil {
		p.op.Frames.Append(ipc.Frame{
			ID:   p.id,
			Type: ipc.FrameError,
			Msg:  err.Error(),
		})
		if p.reg != nil {
			p.reg.Finish(p.id, "error", err)
		}
	} else {
		data, _ := json.Marshal(map[string]any{
			"task_id":     p.id,
			"total_bytes": p.totalB,
			"total_files": p.totalN,
		})
		p.op.Frames.Append(ipc.Frame{
			ID:   p.id,
			Type: ipc.FrameDone,
			Data: data,
		})
		if p.reg != nil {
			p.reg.Finish(p.id, "done", nil)
		}
	}
}
