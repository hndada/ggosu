//go:build !js

package multi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Received struct {
	Event
	At time.Duration // sampled immediately after ReadMessage, on the input QPC clock
}

// Client owns one reader and one writer. Send never waits for network I/O.
// The owner drains Events and calls Close when its game/process ends.
type Client struct {
	conn   *websocket.Conn
	out    chan Request
	events chan Received
	errors chan error
	stop   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
	now    func() time.Duration
}

func Dial(ctx context.Context, endpoint string, now func() time.Duration) (*Client, error) {
	if now == nil {
		return nil, fmt.Errorf("multi: client clock required")
	}
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := d.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("multi: connect: %w", err)
	}
	c := &Client{conn: conn, out: make(chan Request, 64), events: make(chan Received, 128), errors: make(chan error, 1), stop: make(chan struct{}), now: now}
	conn.SetReadLimit(32 << 10)
	c.wg.Add(2)
	go c.read()
	go c.write()
	return c, nil
}

func (c *Client) Events() <-chan Received { return c.events }
func (c *Client) Errors() <-chan error    { return c.errors }

func (c *Client) Send(q Request) error {
	select {
	case <-c.stop:
		return fmt.Errorf("multi: connection closed")
	default:
	}
	select {
	case c.out <- q:
		return nil
	default:
		return fmt.Errorf("multi: outgoing queue full")
	}
}

func (c *Client) fail(err error) {
	select {
	case c.errors <- err:
	default:
	}
	c.stopOnce()
}

func (c *Client) stopOnce() {
	c.once.Do(func() {
		close(c.stop)
		if err := c.conn.Close(); err != nil {
			return
		}
	})
}

func (c *Client) read() {
	defer c.wg.Done()
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		c.fail(err)
		return
	}
	c.conn.SetPingHandler(func(data string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return err
		}
		return c.conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})
	for {
		_, b, err := c.conn.ReadMessage()
		at := c.now()
		if err != nil {
			select {
			case <-c.stop:
			default:
				c.fail(fmt.Errorf("multi: receive: %w", err))
			}
			return
		}
		var e Event
		if err := json.Unmarshal(b, &e); err != nil {
			c.fail(fmt.Errorf("multi: decode event: %w", err))
			return
		}
		select {
		case c.events <- Received{Event: e, At: at}:
		case <-c.stop:
			return
		default:
			c.fail(fmt.Errorf("multi: incoming queue full"))
			return
		}
	}
}

func (c *Client) write() {
	defer c.wg.Done()
	for {
		select {
		case <-c.stop:
			return
		case q := <-c.out:
			if q.Type == "clock" {
				q.SentNS = int64(c.now()) // Exclude time spent in our outgoing queue.
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				c.fail(err)
				return
			}
			if err := c.conn.WriteJSON(q); err != nil {
				c.fail(fmt.Errorf("multi: send: %w", err))
				return
			}
		}
	}
}

func (c *Client) Close() error {
	c.stopOnce()
	c.wg.Wait()
	return nil
}
