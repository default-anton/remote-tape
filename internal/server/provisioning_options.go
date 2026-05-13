package server

import (
	"fmt"
	"slices"
	"strings"
)

type provisioningRegion struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

type provisioningSize struct {
	Slug         string `json:"slug"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Recommended  bool   `json:"recommended"`
	DedicatedCPU bool   `json:"dedicated_cpu"`
}

type provisioningDefaults struct {
	Region string `json:"region"`
	Size   string `json:"size"`
}

type provisioningValidationError struct {
	message string
}

func (e provisioningValidationError) Error() string {
	return e.message
}

func (e provisioningValidationError) Is(target error) bool {
	return target == errInvalidProvisioningSelection
}

type provisioningOptions struct {
	Defaults                provisioningDefaults `json:"defaults"`
	Regions                 []provisioningRegion `json:"regions"`
	Sizes                   []provisioningSize   `json:"sizes"`
	Availability            map[string][]string  `json:"availability"`
	RecommendedSizeByRegion map[string]string    `json:"recommended_size_by_region"`
}

const (
	defaultProvisioningRegion = "nyc3"
	defaultProvisioningSize   = "s-2vcpu-4gb"
)

var errInvalidProvisioningSelection = provisioningValidationError{}

// Static DigitalOcean region and size allowlist, verified as of 2026-05-12.
var provisioningCatalog = provisioningOptions{
	Defaults: provisioningDefaults{Region: defaultProvisioningRegion, Size: defaultProvisioningSize},
	Regions: []provisioningRegion{
		{Slug: "nyc1", Label: "New York 1"},
		{Slug: "nyc2", Label: "New York 2"},
		{Slug: "nyc3", Label: "New York 3"},
		{Slug: "sfo2", Label: "San Francisco 2"},
		{Slug: "sfo3", Label: "San Francisco 3"},
		{Slug: "ams3", Label: "Amsterdam 3"},
		{Slug: "fra1", Label: "Frankfurt 1"},
		{Slug: "lon1", Label: "London 1"},
		{Slug: "blr1", Label: "Bangalore 1"},
		{Slug: "sgp1", Label: "Singapore 1"},
		{Slug: "tor1", Label: "Toronto 1"},
		{Slug: "syd1", Label: "Sydney 1"},
	},
	Sizes: []provisioningSize{
		{Slug: "s-2vcpu-2gb", Label: "Shared CPU Basic", Description: "2 vCPU / 2 GB / 60 GB — low-cost small session", DedicatedCPU: false},
		{Slug: "s-2vcpu-4gb", Label: "Shared CPU Basic", Description: "2 vCPU / 4 GB / 80 GB — recommended default", Recommended: true, DedicatedCPU: false},
		{Slug: "s-4vcpu-8gb", Label: "Shared CPU Basic", Description: "4 vCPU / 8 GB / 160 GB — larger shared session", DedicatedCPU: false},
		{Slug: "c-2", Label: "Dedicated CPU CPU-Optimized", Description: "2 vCPU / 4 GB / 25 GB — recommended production session", Recommended: true, DedicatedCPU: true},
		{Slug: "c-4", Label: "Dedicated CPU CPU-Optimized", Description: "4 vCPU / 8 GB / 50 GB — larger production session", DedicatedCPU: true},
	},
	Availability: map[string][]string{
		"nyc1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb"},
		"nyc2": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb"},
		"nyc3": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb"},
		"sfo2": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"sfo3": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"ams3": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"fra1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb"},
		"lon1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"blr1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"sgp1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"tor1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb", "c-2", "c-4"},
		"syd1": []string{"s-2vcpu-2gb", "s-2vcpu-4gb", "s-4vcpu-8gb"},
	},
	RecommendedSizeByRegion: map[string]string{
		"nyc1": "s-2vcpu-4gb",
		"nyc2": "s-2vcpu-4gb",
		"nyc3": "s-2vcpu-4gb",
		"sfo2": "s-2vcpu-4gb",
		"sfo3": "s-2vcpu-4gb",
		"ams3": "s-2vcpu-4gb",
		"fra1": "s-2vcpu-4gb",
		"lon1": "s-2vcpu-4gb",
		"blr1": "s-2vcpu-4gb",
		"sgp1": "s-2vcpu-4gb",
		"tor1": "s-2vcpu-4gb",
		"syd1": "s-2vcpu-4gb",
	},
}

func provisioningOptionsFor(region, size string) provisioningOptions {
	options := provisioningCatalog
	options.Defaults = provisioningDefaults{Region: region, Size: size}
	return options
}

func validateProvisioningCatalogDefaults(region, size string) error {
	if err := validateProvisioningSelection(region, size); err != nil {
		return fmt.Errorf("configured provisioning defaults are invalid: %w", err)
	}
	return nil
}

func validateProvisioningSelection(region, size string) error {
	region = strings.TrimSpace(region)
	size = strings.TrimSpace(size)
	if !supportedRegion(region) {
		return provisioningValidationError{message: fmt.Sprintf("unsupported instance region %q; choose one of: %s", region, strings.Join(regionSlugs(), ", "))}
	}
	if !supportedSize(size) {
		return provisioningValidationError{message: fmt.Sprintf("unsupported instance size %q; choose one of: %s", size, strings.Join(sizeSlugs(), ", "))}
	}
	if !slices.Contains(provisioningCatalog.Availability[region], size) {
		return provisioningValidationError{message: fmt.Sprintf("instance size %q is not available in region %q", size, region)}
	}
	return nil
}

func supportedRegion(region string) bool {
	return slices.Contains(regionSlugs(), region)
}

func supportedSize(size string) bool {
	return slices.Contains(sizeSlugs(), size)
}

func regionSlugs() []string {
	slugs := make([]string, 0, len(provisioningCatalog.Regions))
	for _, region := range provisioningCatalog.Regions {
		slugs = append(slugs, region.Slug)
	}
	return slugs
}

func sizeSlugs() []string {
	slugs := make([]string, 0, len(provisioningCatalog.Sizes))
	for _, size := range provisioningCatalog.Sizes {
		slugs = append(slugs, size.Slug)
	}
	return slugs
}
