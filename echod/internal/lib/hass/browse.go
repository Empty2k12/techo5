package hass

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Media is one entry of a media browser listing: a station, or a folder of them.
type Media struct {
	Title    string  `json:"title"`
	ID       string  `json:"media_content_id"`
	Kind     string  `json:"media_content_type"`
	CanPlay  bool    `json:"can_play"`
	Thumb    string  `json:"thumbnail"`
	Children []Media `json:"children"`
}

// Browse lists a media source folder, like media-source://radio_browser/local. Home Assistant only
// offers this over its websocket API, so a connection is opened for the one question.
func (c *Client) Browse(ctx context.Context, id string) (Media, error) {
	c.mu.Lock()
	acc := c.acc
	c.mu.Unlock()
	if acc.URL == "" || acc.Token == "" {
		return Media{}, errors.New("hass: no access configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	url := "ws" + strings.TrimPrefix(acc.URL, "http") + "/api/websocket"
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{})
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return Media{}, fmt.Errorf("hass: websocket: %w", err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(dl)
		_ = conn.SetWriteDeadline(dl)
	}

	var hello struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return Media{}, err
	}
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": acc.Token}); err != nil {
		return Media{}, err
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return Media{}, err
	}
	if hello.Type != "auth_ok" {
		return Media{}, fmt.Errorf("hass: websocket auth: %s %s", hello.Type, hello.Message)
	}
	if err := conn.WriteJSON(map[string]any{"id": 1, "type": "media_source/browse_media", "media_content_id": id}); err != nil {
		return Media{}, err
	}
	for {
		var res struct {
			ID      int    `json:"id"`
			Type    string `json:"type"`
			Success bool   `json:"success"`
			Result  Media  `json:"result"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&res); err != nil {
			return Media{}, err
		}
		if res.ID != 1 || res.Type != "result" {
			continue
		}
		if !res.Success {
			return Media{}, fmt.Errorf("hass: browse %s: %s %s", id, res.Error.Code, res.Error.Message)
		}
		return res.Result, nil
	}
}
