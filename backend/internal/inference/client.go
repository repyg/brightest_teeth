package inference

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/repyg/brightest_teeth/backend/api"
	"github.com/repyg/brightest_teeth/backend/internal/mlapi"
)

type Client struct {
	api     *mlapi.ClientWithResponses
	version string
}

type boundedClient struct{ *http.Client }

func (c boundedClient) Do(req *http.Request) (*http.Response, error) {
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body = struct {
		io.Reader
		io.Closer
	}{io.LimitReader(res.Body, 64<<10), res.Body}
	return res, nil
}

func New(endpoint, version string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid ML_URL")
	}
	client, err := mlapi.NewClientWithResponses(endpoint, mlapi.WithHTTPClient(boundedClient{&http.Client{Timeout: 45 * time.Second}}))
	if err != nil {
		return nil, err
	}
	return &Client{client, version}, nil
}

func (c *Client) Ready(ctx context.Context) error {
	response, err := c.api.ReadyWithResponse(ctx)
	if err != nil {
		return err
	}
	if response.JSON200 == nil || response.JSON200.ModelVersion != c.version {
		return fmt.Errorf("ML not ready or model version mismatch")
	}
	return nil
}

func (c *Client) Extract(ctx context.Context, image []byte, bbox api.BBox) (api.Feature, error) {
	body := mlapi.ExtractJSONRequestBody{Image: image}
	body.Bbox.X, body.Bbox.Y, body.Bbox.W, body.Bbox.H = bbox.X, bbox.Y, bbox.W, bbox.H
	response, err := c.api.ExtractWithResponse(ctx, body)
	if err != nil {
		return api.Feature{}, err
	}
	if response.JSON200 == nil {
		return api.Feature{}, fmt.Errorf("ML returned HTTP %d", response.StatusCode())
	}
	return api.Feature{Embedding: response.JSON200.Embedding, ModelVersion: response.JSON200.ModelVersion}, nil
}
