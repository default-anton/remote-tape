package provisioning

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/default-anton/remote-tape/internal/session"
	"github.com/digitalocean/godo"
)

const baseTag = "remote-tape"

type DigitalOceanConfig struct {
	APIToken string
	SSHKeys  []string
}

type DigitalOceanInstanceProvider struct {
	client       *godo.Client
	sshKeys      []godo.DropletCreateSSHKey
	sessionLocks sync.Map
}

func NewDigitalOceanInstanceProvider(cfg DigitalOceanConfig) (*DigitalOceanInstanceProvider, error) {
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("digitalocean api token is required")
	}
	keys, err := parseSSHKeys(cfg.SSHKeys)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("digitalocean ssh keys are required")
	}
	return &DigitalOceanInstanceProvider{client: godo.NewFromToken(cfg.APIToken), sshKeys: keys}, nil
}

func (p *DigitalOceanInstanceProvider) EnsureInstance(ctx context.Context, s session.Session) (InstanceResult, error) {
	unlock := p.lockSession(s.ID)
	defer unlock()
	return p.ensureDroplet(ctx, s)
}

func (p *DigitalOceanInstanceProvider) ensureDroplet(ctx context.Context, s session.Session) (InstanceResult, error) {
	sessionTag := sessionTag(s.ID)
	requiredTags := []string{baseTag, sessionTag}
	for _, tag := range requiredTags {
		if err := p.ensureTag(ctx, tag); err != nil {
			return InstanceResult{}, err
		}
	}

	if s.InstanceID != nil && strings.TrimSpace(*s.InstanceID) != "" {
		id, err := strconv.Atoi(strings.TrimSpace(*s.InstanceID))
		if err != nil {
			return InstanceResult{}, fmt.Errorf("parse persisted digitalocean droplet id %q for session %s: %w", *s.InstanceID, s.ID, err)
		}
		droplet, _, err := p.client.Droplets.Get(ctx, id)
		if err == nil {
			if err := p.repairTags(ctx, droplet.ID, droplet.Tags, requiredTags); err != nil {
				return InstanceResult{}, err
			}
			return InstanceResult{ID: strconv.Itoa(droplet.ID), IP: publicIPv4(*droplet), Adopted: true}, nil
		}
		if !isNotFound(err) {
			return InstanceResult{}, fmt.Errorf("get persisted digitalocean droplet %d for session %s: %w", id, s.ID, err)
		}
	}

	droplets, err := p.listDropletsByTag(ctx, sessionTag)
	if err != nil {
		return InstanceResult{}, err
	}
	if len(droplets) > 0 {
		d := droplets[0]
		if err := p.repairTags(ctx, d.ID, d.Tags, requiredTags); err != nil {
			return InstanceResult{}, err
		}
		return InstanceResult{ID: strconv.Itoa(d.ID), IP: publicIPv4(d), Adopted: true}, nil
	}

	region := strings.TrimSpace(s.InstanceRegion)
	size := strings.TrimSpace(s.InstanceSize)
	droplet, _, err := p.client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:    dropletName(s.Slug),
		Region:  region,
		Size:    size,
		Image:   createImage(s.ImageID),
		Tags:    []string{baseTag, sessionTag},
		SSHKeys: p.sshKeys,
	})
	if err != nil {
		return InstanceResult{}, fmt.Errorf("create digitalocean droplet for session %s in region %q with size %q: %w", s.ID, region, size, err)
	}
	return InstanceResult{ID: strconv.Itoa(droplet.ID), IP: publicIPv4(*droplet)}, nil
}

func (p *DigitalOceanInstanceProvider) ForceDestroySessionServer(ctx context.Context, s session.Session) (DestroyResult, error) {
	unlock := p.lockSession(s.ID)
	defer unlock()
	return p.forceDestroySessionServer(ctx, s)
}

func (p *DigitalOceanInstanceProvider) forceDestroySessionServer(ctx context.Context, s session.Session) (DestroyResult, error) {
	destroyed := ""
	if s.InstanceID != nil && strings.TrimSpace(*s.InstanceID) != "" {
		id, err := strconv.Atoi(strings.TrimSpace(*s.InstanceID))
		if err != nil {
			return DestroyResult{}, fmt.Errorf("parse persisted digitalocean droplet id %q: %w", *s.InstanceID, err)
		}
		if err := p.deleteDroplet(ctx, id); err != nil {
			return DestroyResult{}, fmt.Errorf("delete digitalocean droplet %d: %w", id, err)
		}
		destroyed = strconv.Itoa(id)
	}

	droplets, err := p.listDropletsByTag(ctx, sessionTag(s.ID))
	if err != nil {
		return DestroyResult{}, err
	}
	for _, d := range droplets {
		if err := p.deleteDroplet(ctx, d.ID); err != nil {
			return DestroyResult{}, fmt.Errorf("delete digitalocean droplet %d by session tag: %w", d.ID, err)
		}
		destroyed = strconv.Itoa(d.ID)
	}
	return DestroyResult{InstanceID: destroyed}, nil
}

func (p *DigitalOceanInstanceProvider) lockSession(sessionID string) func() {
	value, _ := p.sessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (p *DigitalOceanInstanceProvider) deleteDroplet(ctx context.Context, id int) error {
	_, err := p.client.Droplets.Delete(ctx, id)
	if err == nil || isNotFound(err) {
		return nil
	}
	return err
}

func (p *DigitalOceanInstanceProvider) ensureTag(ctx context.Context, tag string) error {
	_, _, err := p.client.Tags.Create(ctx, &godo.TagCreateRequest{Name: tag})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("ensure digitalocean tag %q: %w", tag, err)
	}
	return nil
}

func (p *DigitalOceanInstanceProvider) listDropletsByTag(ctx context.Context, tag string) ([]godo.Droplet, error) {
	var all []godo.Droplet
	opt := &godo.ListOptions{PerPage: 100}
	for {
		droplets, resp, err := p.client.Droplets.ListByTag(ctx, tag, opt)
		if err != nil {
			return nil, fmt.Errorf("list digitalocean droplets by tag %q: %w", tag, err)
		}
		all = append(all, droplets...)
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, fmt.Errorf("read digitalocean droplet list page: %w", err)
		}
		opt.Page = page + 1
	}
	return all, nil
}

func (p *DigitalOceanInstanceProvider) repairTags(ctx context.Context, dropletID int, existing []string, required []string) error {
	have := map[string]bool{}
	for _, tag := range existing {
		have[tag] = true
	}
	for _, tag := range required {
		if have[tag] {
			continue
		}
		_, err := p.client.Tags.TagResources(ctx, tag, &godo.TagResourcesRequest{Resources: []godo.Resource{{ID: strconv.Itoa(dropletID), Type: "droplet"}}})
		if err != nil {
			return fmt.Errorf("tag adopted digitalocean droplet %d with %q: %w", dropletID, tag, err)
		}
	}
	return nil
}

func parseSSHKeys(values []string) ([]godo.DropletCreateSSHKey, error) {
	var keys []godo.DropletCreateSSHKey
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, err := strconv.Atoi(value); err == nil {
			keys = append(keys, godo.DropletCreateSSHKey{ID: id})
			continue
		}
		keys = append(keys, godo.DropletCreateSSHKey{Fingerprint: value})
	}
	return keys, nil
}

func createImage(value string) godo.DropletCreateImage {
	value = strings.TrimSpace(value)
	if id, err := strconv.Atoi(value); err == nil {
		return godo.DropletCreateImage{ID: id}
	}
	return godo.DropletCreateImage{Slug: value}
}

func publicIPv4(d godo.Droplet) string {
	for _, network := range d.Networks.V4 {
		if network.Type == "public" && net.ParseIP(network.IPAddress) != nil {
			return network.IPAddress
		}
	}
	return ""
}

func sessionTag(sessionID string) string {
	return "remote-tape-session:" + sessionID
}

func dropletName(slug string) string {
	const maxLen = 63
	name := session.Slugify("remote-tape-" + slug)
	if len(name) > maxLen {
		name = strings.TrimRight(name[:maxLen], "-")
	}
	if name == "" {
		return "remote-tape-session"
	}
	return name
}

func isAlreadyExists(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "already exists") || strings.Contains(strings.ToLower(err.Error()), "unprocessable entity")
}

func isNotFound(err error) bool {
	if e, ok := err.(*godo.ErrorResponse); ok && e.Response != nil && e.Response.StatusCode == 404 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
