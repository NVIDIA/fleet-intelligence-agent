package netutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

// GeolocationService represents a geolocation service provider
type GeolocationService interface {
	GetLocation(ctx context.Context, ip string) (*IPGeolocation, error)
	Name() string
}

// IPGeolocation represents location data from IP geolocation
type IPGeolocation struct {
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	RegionCode  string  `json:"region_code"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
}

// GetIPGeolocation attempts to get geolocation from multiple services
func GetIPGeolocation(ctx context.Context, ip string) (*IPGeolocation, error) {
	if ip == "" {
		return nil, fmt.Errorf("IP address is required")
	}

	services := []GeolocationService{
		NewIPAPIService(),
		NewIPInfoService(),
		NewFreeGeoIPService(),
	}

	var lastErr error
	for _, service := range services {
		location, err := service.GetLocation(ctx, ip)
		if err == nil && location != nil {
			log.Logger.Debugw("successfully got geolocation",
				"service", service.Name(),
				"ip", ip,
				"country", location.Country,
				"region", location.Region,
				"city", location.City)
			return location, nil
		}
		log.Logger.Warnw("geolocation service failed",
			"service", service.Name(),
			"ip", ip,
			"error", err)
		lastErr = err
	}

	return nil, fmt.Errorf("all geolocation services failed, last error: %w", lastErr)
}

// === IP-API Service (Free, no API key required) ===

type IPAPIService struct {
	baseURL string
	client  *http.Client
}

func NewIPAPIService() *IPAPIService {
	return &IPAPIService{
		baseURL: "http://ip-api.com/json",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *IPAPIService) Name() string {
	return "ip-api.com"
}

type ipAPIResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Query       string  `json:"query"`
}

func (s *IPAPIService) GetLocation(ctx context.Context, ip string) (*IPGeolocation, error) {
	url := fmt.Sprintf("%s/%s", s.baseURL, ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp ipAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	return &IPGeolocation{
		IP:          apiResp.Query,
		Country:     apiResp.Country,
		CountryCode: apiResp.CountryCode,
		Region:      apiResp.RegionName,
		RegionCode:  apiResp.Region,
		City:        apiResp.City,
		Latitude:    apiResp.Lat,
		Longitude:   apiResp.Lon,
		Timezone:    apiResp.Timezone,
		ISP:         apiResp.ISP,
	}, nil
}

// === IPInfo Service (Requires API token for higher limits) ===

type IPInfoService struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewIPInfoService() *IPInfoService {
	return &IPInfoService{
		baseURL: "https://ipinfo.io",
		token:   "", // Set via environment variable IPINFO_TOKEN
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *IPInfoService) Name() string {
	return "ipinfo.io"
}

type ipInfoResponse struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	City     string `json:"city"`
	Loc      string `json:"loc"` // "lat,lon"
	Timezone string `json:"timezone"`
	Org      string `json:"org"`
}

func (s *IPInfoService) GetLocation(ctx context.Context, ip string) (*IPGeolocation, error) {
	url := fmt.Sprintf("%s/%s/json", s.baseURL, ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp ipInfoResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	// Parse coordinates from "lat,lon" format
	var lat, lon float64
	if apiResp.Loc != "" {
		fmt.Sscanf(apiResp.Loc, "%f,%f", &lat, &lon)
	}

	return &IPGeolocation{
		IP:          apiResp.IP,
		Country:     "", // IPInfo doesn't provide country name in free tier
		CountryCode: apiResp.Country,
		Region:      apiResp.Region,
		RegionCode:  apiResp.Region,
		City:        apiResp.City,
		Latitude:    lat,
		Longitude:   lon,
		Timezone:    apiResp.Timezone,
		ISP:         apiResp.Org,
	}, nil
}

// === FreeGeoIP Service (Backup service) ===

type FreeGeoIPService struct {
	baseURL string
	client  *http.Client
}

func NewFreeGeoIPService() *FreeGeoIPService {
	return &FreeGeoIPService{
		baseURL: "https://freeipapi.com/api/json",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *FreeGeoIPService) Name() string {
	return "freeipapi.com"
}

type freeGeoIPResponse struct {
	IPVersion     int     `json:"ipVersion"`
	IPAddress     string  `json:"ipAddress"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	CountryName   string  `json:"countryName"`
	CountryCode   string  `json:"countryCode"`
	TimeZone      string  `json:"timeZone"`
	ZipCode       string  `json:"zipCode"`
	CityName      string  `json:"cityName"`
	RegionName    string  `json:"regionName"`
	Continent     string  `json:"continent"`
	ContinentCode string  `json:"continentCode"`
}

func (s *FreeGeoIPService) GetLocation(ctx context.Context, ip string) (*IPGeolocation, error) {
	url := fmt.Sprintf("%s/%s", s.baseURL, ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp freeGeoIPResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	return &IPGeolocation{
		IP:          apiResp.IPAddress,
		Country:     apiResp.CountryName,
		CountryCode: apiResp.CountryCode,
		Region:      apiResp.RegionName,
		RegionCode:  apiResp.RegionName,
		City:        apiResp.CityName,
		Latitude:    apiResp.Latitude,
		Longitude:   apiResp.Longitude,
		Timezone:    apiResp.TimeZone,
		ISP:         "",
	}, nil
}
