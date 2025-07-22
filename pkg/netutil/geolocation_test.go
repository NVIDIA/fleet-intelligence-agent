package netutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIPGeolocation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping geolocation test in short mode")
	}

	ctx := context.Background()

	// Test with multiple known public IPs
	testIPs := []struct {
		ip          string
		description string
	}{
		{"8.8.8.8", "Google DNS"},
		{"1.1.1.1", "Cloudflare DNS"},
		{"208.67.222.222", "OpenDNS"},
		{"9.9.9.9", "Quad9 DNS"},
	}

	t.Logf("=== Testing IP Geolocation with Real Services ===")

	for _, test := range testIPs {
		t.Logf("\n--- Testing %s (%s) ---", test.ip, test.description)

		location, err := GetIPGeolocation(ctx, test.ip)
		if err != nil {
			t.Logf("❌ Failed to get geolocation for %s: %v", test.ip, err)
			continue
		}

		assert.NotNil(t, location)
		assert.Equal(t, test.ip, location.IP)
		assert.NotEmpty(t, location.CountryCode)

		// Print detailed geolocation information
		t.Logf("✅ Geolocation for %s:", test.ip)
		t.Logf("   📍 Country: %s (%s)", location.Country, location.CountryCode)
		t.Logf("   🏙️  Region: %s (%s)", location.Region, location.RegionCode)
		t.Logf("   🌆 City: %s", location.City)
		t.Logf("   🌍 Coordinates: %.4f, %.4f", location.Latitude, location.Longitude)
		t.Logf("   ⏰ Timezone: %s", location.Timezone)
		t.Logf("   🌐 ISP: %s", location.ISP)
	}
}

func TestGetIPGeolocation_EmptyIP(t *testing.T) {
	ctx := context.Background()

	location, err := GetIPGeolocation(ctx, "")
	assert.Error(t, err)
	assert.Nil(t, location)
	assert.Contains(t, err.Error(), "IP address is required")
}

func TestIPAPIService(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"status": "success",
			"country": "United States",
			"countryCode": "US",
			"region": "CA",
			"regionName": "California",
			"city": "Mountain View",
			"lat": 37.4056,
			"lon": -122.0775,
			"timezone": "America/Los_Angeles",
			"isp": "Google LLC",
			"query": "8.8.8.8"
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create service with mock URL
	service := &IPAPIService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	require.NoError(t, err)
	assert.Equal(t, "8.8.8.8", location.IP)
	assert.Equal(t, "United States", location.Country)
	assert.Equal(t, "US", location.CountryCode)
	assert.Equal(t, "California", location.Region)
	assert.Equal(t, "CA", location.RegionCode)
	assert.Equal(t, "Mountain View", location.City)
	assert.Equal(t, 37.4056, location.Latitude)
	assert.Equal(t, -122.0775, location.Longitude)
	assert.Equal(t, "America/Los_Angeles", location.Timezone)
	assert.Equal(t, "Google LLC", location.ISP)
}

func TestIPAPIService_Failure(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"status": "fail",
			"message": "invalid query"
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	service := &IPAPIService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "invalid")

	assert.Error(t, err)
	assert.Nil(t, location)
	assert.Contains(t, err.Error(), "invalid query")
}

func TestIPInfoService(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"ip": "8.8.8.8",
			"country": "US",
			"region": "California",
			"city": "Mountain View",
			"loc": "37.4056,-122.0775",
			"timezone": "America/Los_Angeles",
			"org": "AS15169 Google LLC"
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	service := &IPInfoService{
		baseURL: server.URL,
		token:   "",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	require.NoError(t, err)
	assert.Equal(t, "8.8.8.8", location.IP)
	assert.Equal(t, "US", location.CountryCode)
	assert.Equal(t, "California", location.Region)
	assert.Equal(t, "Mountain View", location.City)
	assert.Equal(t, 37.4056, location.Latitude)
	assert.Equal(t, -122.0775, location.Longitude)
	assert.Equal(t, "America/Los_Angeles", location.Timezone)
	assert.Equal(t, "AS15169 Google LLC", location.ISP)
}

func TestFreeGeoIPService(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"ipVersion": 4,
			"ipAddress": "8.8.8.8",
			"latitude": 37.4056,
			"longitude": -122.0775,
			"countryName": "United States",
			"countryCode": "US",
			"timeZone": "America/Los_Angeles",
			"zipCode": "94043",
			"cityName": "Mountain View",
			"regionName": "California",
			"continent": "North America",
			"continentCode": "NA"
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	service := &FreeGeoIPService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	require.NoError(t, err)
	assert.Equal(t, "8.8.8.8", location.IP)
	assert.Equal(t, "United States", location.Country)
	assert.Equal(t, "US", location.CountryCode)
	assert.Equal(t, "California", location.Region)
	assert.Equal(t, "Mountain View", location.City)
	assert.Equal(t, 37.4056, location.Latitude)
	assert.Equal(t, -122.0775, location.Longitude)
	assert.Equal(t, "America/Los_Angeles", location.Timezone)
}

func TestGeolocationService_Timeout(t *testing.T) {
	// Create a server with a long delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than client timeout
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	service := &IPAPIService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 1 * time.Second, // Short timeout
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	assert.Error(t, err)
	assert.Nil(t, location)
}

func TestGeolocationService_HTTPError(t *testing.T) {
	// Create a server that returns HTTP 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	service := &IPAPIService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	assert.Error(t, err)
	assert.Nil(t, location)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestGeolocationService_InvalidJSON(t *testing.T) {
	// Create a server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": json`)) // Malformed JSON
	}))
	defer server.Close()

	service := &IPAPIService{
		baseURL: server.URL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	ctx := context.Background()
	location, err := service.GetLocation(ctx, "8.8.8.8")

	assert.Error(t, err)
	assert.Nil(t, location)
}

func TestGeolocationService_Names(t *testing.T) {
	assert.Equal(t, "ip-api.com", NewIPAPIService().Name())
	assert.Equal(t, "ipinfo.io", NewIPInfoService().Name())
	assert.Equal(t, "freeipapi.com", NewFreeGeoIPService().Name())
}

// TestIndividualGeolocationServices tests each service individually
func TestIndividualGeolocationServices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping individual geolocation service test in short mode")
	}

	ctx := context.Background()
	testIP := "8.8.8.8" // Google DNS

	services := []GeolocationService{
		NewIPAPIService(),
		NewIPInfoService(),
		NewFreeGeoIPService(),
	}

	t.Logf("\n=== Testing Individual Geolocation Services ===")
	t.Logf("Test IP: %s (Google DNS)\n", testIP)

	for _, service := range services {
		t.Logf("🔍 Testing service: %s", service.Name())

		location, err := service.GetLocation(ctx, testIP)
		if err != nil {
			t.Logf("❌ %s failed: %v\n", service.Name(), err)
			continue
		}

		t.Logf("✅ %s succeeded:", service.Name())
		t.Logf("   📍 Country: %s (%s)", location.Country, location.CountryCode)
		t.Logf("   🏙️  Region: %s (%s)", location.Region, location.RegionCode)
		t.Logf("   🌆 City: %s", location.City)
		t.Logf("   🌍 Coordinates: %.4f, %.4f", location.Latitude, location.Longitude)
		t.Logf("   ⏰ Timezone: %s", location.Timezone)
		t.Logf("   🌐 ISP: %s\n", location.ISP)
	}
}

// TestCurrentIPGeolocation demonstrates geolocating your own public IP
func TestCurrentIPGeolocation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping current IP geolocation test in short mode")
	}

	ctx := context.Background()

	t.Logf("\n=== Testing Current Machine's IP Geolocation ===")

	// Get the machine's public IP first
	publicIP, err := PublicIP()
	if err != nil {
		t.Logf("❌ Failed to get public IP: %v", err)
		t.Skip("Cannot get public IP, skipping test")
	}

	t.Logf("🌐 Your public IP: %s", publicIP)

	// Now geolocate it
	location, err := GetIPGeolocation(ctx, publicIP)
	if err != nil {
		t.Logf("❌ Failed to geolocate your IP: %v", err)
		return
	}

	t.Logf("\n✅ Your location based on IP geolocation:")
	t.Logf("   📍 Country: %s (%s)", location.Country, location.CountryCode)
	t.Logf("   🏙️  Region: %s (%s)", location.Region, location.RegionCode)
	t.Logf("   🌆 City: %s", location.City)
	t.Logf("   🌍 Coordinates: %.4f, %.4f", location.Latitude, location.Longitude)
	t.Logf("   ⏰ Timezone: %s", location.Timezone)
	t.Logf("   🌐 ISP: %s", location.ISP)

	// Assertions for the test framework
	assert.NotNil(t, location)
	assert.Equal(t, publicIP, location.IP)
	assert.NotEmpty(t, location.CountryCode)
}
