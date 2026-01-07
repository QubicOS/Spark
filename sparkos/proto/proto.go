package proto

// Kind identifies the message type carried in kernel.Message.Kind.
type Kind uint16

const (
	MsgLogLine Kind = iota + 1
	MsgSleep
	MsgWake
	MsgError
	MsgTermWrite
	MsgTermClear
	MsgTermInput
	MsgVFSList
	MsgVFSListResp
	MsgVFSMkdir
	MsgVFSMkdirResp
	MsgVFSStat
	MsgVFSStatResp
	MsgVFSRead
	MsgVFSReadResp
	MsgVFSWriteOpen
	MsgVFSWriteChunk
	MsgVFSWriteClose
	MsgVFSWriteResp
	MsgTermRefresh
	MsgAppControl
	MsgAppSelect
	MsgAppShutdown
	MsgVFSRemove
	MsgVFSRemoveResp
	MsgVFSRename
	MsgVFSRenameResp
	MsgAudioSubscribe
	MsgAudioPlay
	MsgAudioPause
	MsgAudioStop
	MsgAudioSetVolume
	MsgAudioStatus
	MsgAudioMeters
)

// ErrCode is a generic error category for MsgError responses.
type ErrCode uint16

const (
	ErrUnknown ErrCode = iota
	ErrBadMessage
	ErrUnauthorized
	ErrNotFound
	ErrBusy
	ErrOverflow
	ErrTooLarge
	ErrInternal
)

func (c ErrCode) String() string {
	switch c {
	case ErrUnknown:
		return "unknown"
	case ErrBadMessage:
		return "bad_message"
	case ErrUnauthorized:
		return "unauthorized"
	case ErrNotFound:
		return "not_found"
	case ErrBusy:
		return "busy"
	case ErrOverflow:
		return "overflow"
	case ErrTooLarge:
		return "too_large"
	case ErrInternal:
		return "internal"
	default:
		return "unknown"
	}
}

func (k Kind) String() string {
	switch k {
	case MsgLogLine:
		return "log_line"
	case MsgSleep:
		return "sleep"
	case MsgWake:
		return "wake"
	case MsgError:
		return "error"
	case MsgTermWrite:
		return "term_write"
	case MsgTermClear:
		return "term_clear"
	case MsgTermInput:
		return "term_input"
	case MsgVFSList:
		return "vfs_list"
	case MsgVFSListResp:
		return "vfs_list_resp"
	case MsgVFSMkdir:
		return "vfs_mkdir"
	case MsgVFSMkdirResp:
		return "vfs_mkdir_resp"
	case MsgVFSStat:
		return "vfs_stat"
	case MsgVFSStatResp:
		return "vfs_stat_resp"
	case MsgVFSRead:
		return "vfs_read"
	case MsgVFSReadResp:
		return "vfs_read_resp"
	case MsgVFSWriteOpen:
		return "vfs_write_open"
	case MsgVFSWriteChunk:
		return "vfs_write_chunk"
	case MsgVFSWriteClose:
		return "vfs_write_close"
	case MsgVFSWriteResp:
		return "vfs_write_resp"
	case MsgTermRefresh:
		return "term_refresh"
	case MsgAppControl:
		return "app_control"
	case MsgAppSelect:
		return "app_select"
	case MsgAppShutdown:
		return "app_shutdown"
	case MsgVFSRemove:
		return "vfs_remove"
	case MsgVFSRemoveResp:
		return "vfs_remove_resp"
	case MsgVFSRename:
		return "vfs_rename"
	case MsgVFSRenameResp:
		return "vfs_rename_resp"
	case MsgAudioSubscribe:
		return "audio_subscribe"
	case MsgAudioPlay:
		return "audio_play"
	case MsgAudioPause:
		return "audio_pause"
	case MsgAudioStop:
		return "audio_stop"
	case MsgAudioSetVolume:
		return "audio_set_volume"
	case MsgAudioStatus:
		return "audio_status"
	case MsgAudioMeters:
		return "audio_meters"
	default:
		return "unknown"
	}
}
