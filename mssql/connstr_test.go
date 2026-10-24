package mssql

import (
	"testing"

	"docklog/db"

	mssqldriver "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
)

func TestBuildDSNTrustSkipsVerify(t *testing.T) {
	s := db.MSSQLSettings{
		Host:                   "127.0.0.1",
		Port:                   1433,
		Username:               "sa",
		Password:               "x",
		DatabaseName:           "container_logs",
		TrustServerCertificate: true,
	}
	dsn, err := buildDSN(s)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := msdsn.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyTrustTLS(&cfg, effectiveTrust(s)); err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("expected TLSConfig")
	}
	if !cfg.TLSConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify=false, dsn=%q", dsn)
	}
	_ = mssqldriver.NewConnectorConfig(cfg)
}
