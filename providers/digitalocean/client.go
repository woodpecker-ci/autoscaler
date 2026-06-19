package digitalocean

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultBaseURL = "https://api.digitalocean.com/v2"
	perPage        = 200
)

type client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

type region struct {
	Slug      string   `json:"slug"`
	Available bool     `json:"available"`
	Sizes     []string `json:"sizes"`
}

type size struct {
	Slug      string   `json:"slug"`
	Regions   []string `json:"regions"`
	Available bool     `json:"available"`
}

type image struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Distribution string `json:"distribution"`
}

type sshKey struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

type droplet struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type dropletCreateImage struct {
	ID   int    `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type dropletCreateRequest struct {
	Name     string             `json:"name"`
	Region   string             `json:"region"`
	Size     string             `json:"size"`
	Image    dropletCreateImage `json:"image"`
	SSHKeys  []string           `json:"ssh_keys,omitempty"`
	Backups  bool               `json:"backups"`
	IPv6     bool               `json:"ipv6"`
	Tags     []string           `json:"tags,omitempty"`
	UserData string             `json:"user_data,omitempty"`
}

func newClient(httpClient *http.Client, baseURL, token string) *client {
	return &client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (c *client) listRegions(ctx context.Context) ([]region, error) {
	var all []region
	for page := 1; ; page++ {
		var resp struct {
			Regions []region `json:"regions"`
		}
		if err := c.doJSON(ctx, http.MethodGet, withPage("/regions", page), nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Regions...)
		if len(resp.Regions) < perPage {
			return all, nil
		}
	}
}

func (c *client) listSizes(ctx context.Context) ([]size, error) {
	var all []size
	for page := 1; ; page++ {
		var resp struct {
			Sizes []size `json:"sizes"`
		}
		if err := c.doJSON(ctx, http.MethodGet, withPage("/sizes", page), nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Sizes...)
		if len(resp.Sizes) < perPage {
			return all, nil
		}
	}
}

func (c *client) listImages(ctx context.Context) ([]image, error) {
	var all []image
	for page := 1; ; page++ {
		var resp struct {
			Images []image `json:"images"`
		}
		if err := c.doJSON(ctx, http.MethodGet, withPage("/images", page), nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Images...)
		if len(resp.Images) < perPage {
			return all, nil
		}
	}
}

func (c *client) listSSHKeys(ctx context.Context) ([]sshKey, error) {
	var all []sshKey
	for page := 1; ; page++ {
		var resp struct {
			SSHKeys []sshKey `json:"ssh_keys"`
		}
		if err := c.doJSON(ctx, http.MethodGet, withPage("/account/keys", page), nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.SSHKeys...)
		if len(resp.SSHKeys) < perPage {
			return all, nil
		}
	}
}

func (c *client) listDropletsByTag(ctx context.Context, tag string) ([]droplet, error) {
	var all []droplet
	for page := 1; ; page++ {
		path := withPage("/droplets?tag_name="+url.QueryEscape(tag), page)
		var resp struct {
			Droplets []droplet `json:"droplets"`
		}
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Droplets...)
		if len(resp.Droplets) < perPage {
			return all, nil
		}
	}
}

func (c *client) createDroplet(ctx context.Context, req dropletCreateRequest) (*droplet, error) {
	var resp struct {
		Droplet droplet `json:"droplet"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/droplets", req, &resp); err != nil {
		return nil, err
	}

	return &resp.Droplet, nil
}

func (c *client) deleteDroplet(ctx context.Context, id int) error {
	return c.doJSON(ctx, http.MethodDelete, "/droplets/"+strconv.Itoa(id), nil, nil)
}

func (c *client) doJSON(ctx context.Context, method, path string, reqBody any, dest any) error {
	var body io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if dest == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

func withPage(path string, page int) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "page=" + strconv.Itoa(page) + "&per_page=" + strconv.Itoa(perPage)
}
