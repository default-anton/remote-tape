package server

import (
	"strings"
	"testing"
)

func TestProvisioningOptionsIncludeDevOnlyCheapestSizeOnlyInDevelopment(t *testing.T) {
	dev := provisioningOptionsFor(provisioningEnvironmentDevelopment, defaultProvisioningRegion, defaultProvisioningSize)
	if !hasProvisioningSize(dev, devOnlyCheapestProvisioningSize) {
		t.Fatalf("development sizes do not include %q: %+v", devOnlyCheapestProvisioningSize, dev.Sizes)
	}
	if !hasAvailableProvisioningSize(dev, "nyc3", devOnlyCheapestProvisioningSize) {
		t.Fatalf("development nyc3 availability does not include %q: %+v", devOnlyCheapestProvisioningSize, dev.Availability["nyc3"])
	}

	prod := provisioningOptionsFor(provisioningEnvironmentProduction, defaultProvisioningRegion, defaultProvisioningSize)
	if hasProvisioningSize(prod, devOnlyCheapestProvisioningSize) {
		t.Fatalf("production sizes include development-only size: %+v", prod.Sizes)
	}
	if hasAvailableProvisioningSize(prod, "nyc3", devOnlyCheapestProvisioningSize) {
		t.Fatalf("production nyc3 availability includes development-only size: %+v", prod.Availability["nyc3"])
	}
	if prod.Defaults.Size != defaultProvisioningSize || prod.RecommendedSizeByRegion["nyc3"] != defaultProvisioningSize {
		t.Fatalf("production defaults changed: defaults=%+v recommended=%+v", prod.Defaults, prod.RecommendedSizeByRegion)
	}
}

func TestValidateProvisioningSelectionGuardsDevOnlyCheapestSize(t *testing.T) {
	if err := validateProvisioningSelection(provisioningEnvironmentDevelopment, "nyc3", devOnlyCheapestProvisioningSize); err != nil {
		t.Fatalf("development validation error = %v", err)
	}

	err := validateProvisioningSelection(provisioningEnvironmentProduction, "nyc3", devOnlyCheapestProvisioningSize)
	if err == nil || !strings.Contains(err.Error(), `instance size "s-1vcpu-512mb-10gb" is development-only`) {
		t.Fatalf("production validation error = %v", err)
	}
}

func hasProvisioningSize(options provisioningOptions, slug string) bool {
	for _, size := range options.Sizes {
		if size.Slug == slug {
			return true
		}
	}
	return false
}

func hasAvailableProvisioningSize(options provisioningOptions, region string, slug string) bool {
	for _, size := range options.Availability[region] {
		if size == slug {
			return true
		}
	}
	return false
}
