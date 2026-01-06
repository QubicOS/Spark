package proto

// AppID identifies a foreground app in the console multiplexer.
type AppID uint8

const (
	AppNone    AppID = 0
	AppRTDemo  AppID = 1
	AppVi      AppID = 2
	AppRTVoxel AppID = 3
	AppImgView AppID = 4
	AppMC      AppID = 5
	AppHex     AppID = 6
	AppVector  AppID = 7
)

// AppSelectPayload encodes an app selection request.
//
// Payload format:
//
//	b[0]   : AppID
//	b[1:]  : optional UTF-8 argument (app-defined)
func AppSelectPayload(id AppID, arg string) []byte {
	b := make([]byte, 1, 1+len(arg))
	b[0] = byte(id)
	b = append(b, []byte(arg)...)
	return b
}

func DecodeAppSelectPayload(b []byte) (id AppID, arg string, ok bool) {
	if len(b) < 1 {
		return 0, "", false
	}
	return AppID(b[0]), string(b[1:]), true
}
