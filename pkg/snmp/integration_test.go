//go:build integration

package snmp

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integration tests require real OLT devices.
// Run with: go test -tags=integration -v ./pkg/snmp/...
//
// Required environment variables:
// - VSOL_OLT_HOST: V-Sol OLT IP address
// - VSOL_OLT_COMMUNITY: SNMP community string
// - HUAWEI_OLT_HOST: Huawei OLT IP address
// - HUAWEI_OLT_COMMUNITY: SNMP community string

func TestVSOLCollector_Integration(t *testing.T) {
	host := os.Getenv("VSOL_OLT_HOST")
	community := os.Getenv("VSOL_OLT_COMMUNITY")

	if host == "" || community == "" {
		t.Skip("VSOL_OLT_HOST and VSOL_OLT_COMMUNITY environment variables required")
	}

	config := DeviceConfig{
		Host:      host,
		Port:      161,
		Community: community,
		Vendor:    VendorVSOL,
		Timeout:   30 * time.Second,
	}

	collector := NewVSOLCollector(config)
	if err := collector.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer collector.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("CollectOLTInfo", func(t *testing.T) {
		info, err := collector.CollectOLTInfo(ctx)
		if err != nil {
			t.Errorf("CollectOLTInfo failed: %v", err)
			return
		}
		if info == nil {
			t.Error("CollectOLTInfo returned nil")
			return
		}
		t.Logf("OLT Info: Host=%s, CPU=%.1f%%, Memory=%.1f%%, Temp=%.1f°C",
			info.Host, info.CPUPercent, info.MemoryPercent, info.Temperature)
	})

	t.Run("CollectPONPorts", func(t *testing.T) {
		ports, err := collector.CollectPONPorts(ctx)
		if err != nil {
			t.Errorf("CollectPONPorts failed: %v", err)
			return
		}
		t.Logf("Found %d PON ports", len(ports))
		for _, p := range ports {
			t.Logf("  Port %s: Status=%s, ONUs=%d", p.Name, p.Status, p.ONUCount)
		}
	})

	t.Run("CollectONUs", func(t *testing.T) {
		onus, err := collector.CollectONUs(ctx)
		if err != nil {
			t.Errorf("CollectONUs failed: %v", err)
			return
		}
		t.Logf("Found %d ONUs", len(onus))
		for i, onu := range onus {
			if i < 5 { // Only log first 5
				t.Logf("  ONU %s: SN=%s, Status=%s, Distance=%dm",
					onu.OnuID, onu.SerialNumber, onu.Status, onu.Distance)
			}
		}
	})

	t.Run("CollectONUOptical", func(t *testing.T) {
		optical, err := collector.CollectONUOptical(ctx)
		if err != nil {
			t.Errorf("CollectONUOptical failed: %v", err)
			return
		}
		t.Logf("Found optical data for %d ONUs", len(optical))
		for i, opt := range optical {
			if i < 5 { // Only log first 5
				t.Logf("  ONU %s: RX=%.2f dBm, TX=%.2f dBm, Status=%s",
					opt.OnuID, opt.RxPowerDBm, opt.TxPowerDBm, opt.Status)
			}
		}
	})

	t.Run("CollectONUTraffic", func(t *testing.T) {
		traffic, err := collector.CollectONUTraffic(ctx)
		if err != nil {
			t.Errorf("CollectONUTraffic failed: %v", err)
			return
		}
		t.Logf("Found traffic data for %d ONUs", len(traffic))
		for i, tr := range traffic {
			if i < 5 { // Only log first 5
				t.Logf("  ONU %s: RX=%d bytes, TX=%d bytes",
					tr.OnuID, tr.RxBytes, tr.TxBytes)
			}
		}
	})

	t.Run("CollectUnauthONUs", func(t *testing.T) {
		unauth, err := collector.CollectUnauthONUs(ctx)
		if err != nil {
			t.Errorf("CollectUnauthONUs failed: %v", err)
			return
		}
		t.Logf("Found %d unauthorized ONUs", len(unauth))
		for _, u := range unauth {
			t.Logf("  Unauth ONU: SN=%s, MAC=%s", u.SerialNumber, u.MAC)
		}
	})

	t.Run("CollectAll", func(t *testing.T) {
		telemetry, err := collector.CollectAll(ctx)
		if err != nil {
			t.Errorf("CollectAll failed: %v", err)
			return
		}
		if telemetry == nil {
			t.Error("CollectAll returned nil")
			return
		}
		t.Logf("CollectAll completed in %v", telemetry.Duration)
		t.Logf("  ONUs: %d, Optical: %d, Traffic: %d, Unauth: %d",
			len(telemetry.ONUs),
			len(telemetry.ONUOptical),
			len(telemetry.ONUTraffic),
			len(telemetry.UnauthONUs))
		if len(telemetry.Errors) > 0 {
			t.Logf("  Errors: %v", telemetry.Errors)
		}
	})
}

func TestHuaweiCollector_Integration(t *testing.T) {
	host := os.Getenv("HUAWEI_OLT_HOST")
	community := os.Getenv("HUAWEI_OLT_COMMUNITY")

	if host == "" || community == "" {
		t.Skip("HUAWEI_OLT_HOST and HUAWEI_OLT_COMMUNITY environment variables required")
	}

	config := DeviceConfig{
		Host:      host,
		Port:      161,
		Community: community,
		Vendor:    VendorHuawei,
		Timeout:   30 * time.Second,
	}

	collector := NewHuaweiCollector(config)
	if err := collector.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer collector.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("CollectOLTInfo", func(t *testing.T) {
		info, err := collector.CollectOLTInfo(ctx)
		if err != nil {
			t.Errorf("CollectOLTInfo failed: %v", err)
			return
		}
		if info == nil {
			t.Error("CollectOLTInfo returned nil")
			return
		}
		t.Logf("OLT Info: Host=%s, MAC=%s, Version=%s",
			info.Host, info.MAC, info.SoftwareVersion)
	})

	t.Run("CollectONUs", func(t *testing.T) {
		onus, err := collector.CollectONUs(ctx)
		if err != nil {
			t.Errorf("CollectONUs failed: %v", err)
			return
		}
		t.Logf("Found %d ONTs", len(onus))
		for i, onu := range onus {
			if i < 5 {
				t.Logf("  ONT %s: SN=%s, Status=%s, Distance=%dm",
					onu.OnuID, onu.SerialNumber, onu.Status, onu.Distance)
			}
		}
	})

	t.Run("CollectONUOptical", func(t *testing.T) {
		optical, err := collector.CollectONUOptical(ctx)
		if err != nil {
			t.Errorf("CollectONUOptical failed: %v", err)
			return
		}
		t.Logf("Found optical data for %d ONTs", len(optical))
	})

	t.Run("CollectUnauthONUs", func(t *testing.T) {
		unauth, err := collector.CollectUnauthONUs(ctx)
		if err != nil {
			t.Errorf("CollectUnauthONUs failed: %v", err)
			return
		}
		t.Logf("Found %d unauthorized ONTs", len(unauth))
		for _, u := range unauth {
			t.Logf("  Unauth ONT: SN=%s, Type=%s", u.SerialNumber, u.Type)
		}
	})

	t.Run("CollectAll", func(t *testing.T) {
		telemetry, err := collector.CollectAll(ctx)
		if err != nil {
			t.Errorf("CollectAll failed: %v", err)
			return
		}
		t.Logf("CollectAll completed in %v", telemetry.Duration)
		t.Logf("  ONTs: %d, Optical: %d, Unauth: %d",
			len(telemetry.ONUs),
			len(telemetry.ONUOptical),
			len(telemetry.UnauthONUs))
		if len(telemetry.Errors) > 0 {
			t.Logf("  Errors: %v", telemetry.Errors)
		}
	})
}
