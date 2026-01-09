package audio

import (
	"fmt"

	"spark/sparkos/kernel"
	"spark/sparkos/proto"
)

type Client struct {
	audioCap kernel.Capability
}

func New(audioCap kernel.Capability) *Client {
	return &Client{audioCap: audioCap}
}

func (c *Client) Subscribe(ctx *kernel.Context, statusCap kernel.Capability) error {
	return c.send(ctx, proto.MsgAudioSubscribe, proto.AudioSubscribePayload(), statusCap)
}

func (c *Client) Play(ctx *kernel.Context, path string, loop bool) error {
	return c.send(ctx, proto.MsgAudioPlay, proto.AudioPlayPayload(loop, path), kernel.Capability{})
}

func (c *Client) Pause(ctx *kernel.Context) error {
	return c.send(ctx, proto.MsgAudioPause, nil, kernel.Capability{})
}

func (c *Client) Stop(ctx *kernel.Context) error {
	return c.send(ctx, proto.MsgAudioStop, nil, kernel.Capability{})
}

func (c *Client) SetVolume(ctx *kernel.Context, vol uint8) error {
	return c.send(ctx, proto.MsgAudioSetVolume, proto.AudioSetVolumePayload(vol), kernel.Capability{})
}

func (c *Client) send(ctx *kernel.Context, kind proto.Kind, payload []byte, xfer kernel.Capability) error {
	if ctx == nil {
		return fmt.Errorf("audio client: nil context for %s", kind)
	}
	if !c.audioCap.Valid() {
		return fmt.Errorf("audio client: missing capability for %s", kind)
	}
	const retryLimit = 500
	retries := 0
	for {
		res := ctx.SendToCapResult(c.audioCap, uint16(kind), payload, xfer)
		switch res {
		case kernel.SendOK:
			return nil
		case kernel.SendErrQueueFull:
			retries++
			if retries >= retryLimit {
				return fmt.Errorf("audio client send %s: queue full", kind)
			}
			ctx.BlockOnTick()
		default:
			return fmt.Errorf("audio client send %s: %s", kind, res)
		}
	}
}
