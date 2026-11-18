package metro

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Reidevl/rn-lens/internal/fastrefresh"
)

// hmrReadLimit bounds one /hot message: updates carry the changed modules'
// code.
const hmrReadLimit = 64 << 20

// HMRDialer opens Metro's /hot WebSocket for the Fast Refresh tracker.
type HMRDialer struct{}

// DialHMR connects to url (ws://host:port/hot).
func (HMRDialer) DialHMR(ctx context.Context, url string) (fastrefresh.Stream, error) {
	c, resp, err := websocket.Dial(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("metro: dial %s: %w", url, err)
	}
	c.SetReadLimit(hmrReadLimit)
	return hmrStream{c}, nil
}

type hmrStream struct{ c *websocket.Conn }

func (s hmrStream) Send(ctx context.Context, v any) error { return wsjson.Write(ctx, s.c, v) }

func (s hmrStream) Recv(ctx context.Context) ([]byte, error) {
	var raw json.RawMessage
	if err := wsjson.Read(ctx, s.c, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s hmrStream) Close() error { return s.c.Close(websocket.StatusNormalClosure, "") }
