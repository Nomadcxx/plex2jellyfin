package ipc

import "encoding/json"

const ProtocolVersion = 1

type Command string

const (
	CmdStatus            Command = "STATUS"
	CmdReload            Command = "RELOAD"
	CmdStop              Command = "STOP"
	CmdRescan            Command = "RESCAN"
	CmdResetDB           Command = "RESET_DB"
	CmdAttach            Command = "ATTACH"
	CmdCancel            Command = "CANCEL"
	CmdRecover           Command = "RECOVER"
	CmdListOps           Command = "LIST_OPS"
	CmdDeferred          Command = "DEFERRED"
	CmdConsolidate       Command = "CONSOLIDATE"
	CmdDupScan           Command = "DUP_SCAN"
	CmdAIBatch           Command = "AI_BATCH"
	CmdMetadataRefresh   Command = "METADATA_REFRESH"
	CmdMetadataReconcile Command = "METADATA_RECONCILE"
	CmdMetadataRepair    Command = "METADATA_REPAIR"
	CmdSweep             Command = "SWEEP"
	CmdParsesAudit       Command = "PARSES_AUDIT"
	CmdJobsList          Command = "JOBS_LIST"
	CmdJobRun            Command = "JOB_RUN"
	CmdJobStop           Command = "JOB_STOP"
	CmdJobUpdate         Command = "JOB_UPDATE"
	CmdTasksList         Command = "TASKS_LIST"
	CmdTaskRetry         Command = "TASK_RETRY"
	CmdTaskCancel        Command = "TASK_CANCEL"
	CmdVerifyFlagged     Command = "VERIFY_FLAGGED"
	CmdTaskGet           Command = "TASK_GET"
	CmdTasksBulk         Command = "TASKS_BULK"
	CmdTasksPurge        Command = "TASKS_PURGE"
	CmdTaskVerify        Command = "TASK_VERIFY"
	CmdTaskGroup         Command = "TASK_GROUP"
	CmdTaskApprove       Command = "TASK_APPROVE"
)

type Request struct {
	V    int             `json:"v"`
	ID   string          `json:"id"`
	Cmd  Command         `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type FrameType string

const (
	FrameResult    FrameType = "result"
	FrameProgress  FrameType = "progress"
	FrameHeartbeat FrameType = "heartbeat"
	FrameDone      FrameType = "done"
	FrameError     FrameType = "error"
)

type Frame struct {
	ID      string          `json:"id"`
	Type    FrameType       `json:"type"`
	Code    ErrorCode       `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Phase   string          `json:"phase,omitempty"`
	Current int             `json:"current,omitempty"`
	Total   int             `json:"total,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	TS      int64           `json:"ts,omitempty"`
}

type ErrorCode string

const (
	ErrBusy            ErrorCode = "BUSY"
	ErrBadRequest      ErrorCode = "BAD_REQUEST"
	ErrVersionMismatch ErrorCode = "VERSION_MISMATCH"
	ErrUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrNotFound        ErrorCode = "NOT_FOUND"
	ErrConflict        ErrorCode = "CONFLICT"
	ErrInterrupted     ErrorCode = "INTERRUPTED"
	ErrCancelled       ErrorCode = "CANCELLED"
	ErrTimeout         ErrorCode = "TIMEOUT"
	ErrInternal        ErrorCode = "INTERNAL"
	ErrNotImplemented  ErrorCode = "NOT_IMPLEMENTED"
)
