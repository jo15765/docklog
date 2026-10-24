package mssql

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"docklog/db"

	mssqldriver "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
)

func effectiveTrust(s db.MSSQLSettings) bool {
	if s.EncryptDisable || strings.EqualFold(os.Getenv("MSSQL_ENCRYPT"), "disable") {
		return false
	}
	switch os.Getenv("MSSQL_TRUST_SERVER_CERTIFICATE") {
	case "true":
		return true
	case "false":
		return false
	}
	return s.TrustServerCertificate
}

func buildDSN(s db.MSSQLSettings) (string, error) {
	db.ApplyMSSQLDefaults(&s)

	host := strings.TrimSpace(s.Host)
	if host == "" {
		return "", errRequired("host")
	}
	port := s.Port
	if port <= 0 {
		port = 1433
	}
	user := strings.TrimSpace(s.Username)
	if user == "" {
		return "", errRequired("username")
	}
	if strings.TrimSpace(s.Password) == "" {
		return "", errRequired("password")
	}
	database := strings.TrimSpace(s.DatabaseName)
	if database == "" {
		return "", errRequired("database")
	}

	trust := effectiveTrust(s)
	encrypt := "true"
	if s.EncryptDisable || strings.EqualFold(os.Getenv("MSSQL_ENCRYPT"), "disable") {
		encrypt = "disable"
	}

	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, s.Password),
		Host:   hostPort,
	}
	q := u.Query()
	q.Set("database", database)
	q.Set("encrypt", encrypt)
	if trust {
		q.Set("TrustServerCertificate", "true")
		q.Set("trust server certificate", "yes")
	} else {
		q.Set("TrustServerCertificate", "false")
	}
	q.Set("connection timeout", "30")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func applyTrustTLS(cfg *msdsn.Config, trust bool) error {
	cfg.TrustServerCertificate = trust
	if cfg.Encryption == msdsn.EncryptionDisabled {
		cfg.TLSConfig = nil
		return nil
	}
	hostInCert := cfg.Host
	if ip := net.ParseIP(strings.Trim(cfg.Host, "[]")); ip != nil {
		hostInCert = "Server"
		cfg.HostInCertificateProvided = true
	}
	tlsConfig, err := msdsn.SetupTLS("", "", trust, hostInCert, "")
	if err != nil {
		return err
	}
	if trust {
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = nil
		tlsConfig.VerifyPeerCertificate = nil
	}
	cfg.TLSConfig = tlsConfig
	return nil
}

func openSQLDB(s db.MSSQLSettings) (*sql.DB, error) {
	dsn, err := buildDSN(s)
	if err != nil {
		return nil, err
	}
	cfg, err := msdsn.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid SQL connection settings: %w", err)
	}
	trust := effectiveTrust(s)
	if err := applyTrustTLS(&cfg, trust); err != nil {
		return nil, fmt.Errorf("TLS setup failed: %w", err)
	}
	connector := mssqldriver.NewConnectorConfig(cfg)
	conn := sql.OpenDB(connector)
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxLifetime(5 * time.Minute)
	return conn, nil
}

type requiredFieldError string

func (e requiredFieldError) Error() string {
	return string(e) + " is required"
}

func errRequired(field string) error {
	return requiredFieldError(field)
}

func ConnectionHint(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "tls") || strings.Contains(msg, "handshake") || strings.Contains(msg, "certificate") {
		return " Enable “Trust server certificate” below, or use “Unencrypted connection” for legacy SQL."
	}
	if strings.Contains(msg, "login error") || strings.Contains(msg, "authentication") || strings.Contains(msg, "18456") {
		return " Check SQL username and password."
	}
	if strings.Contains(msg, "network") || strings.Contains(msg, "timeout") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") || strings.Contains(msg, "could not open") || strings.Contains(msg, "dial tcp") {
		return " Check host and port (default 1433), that SQL Server is running, and TCP is enabled."
	}
	if strings.Contains(msg, "driver error") || strings.Contains(msg, "panic") {
		return " Verify host/port and try “Unencrypted connection” if this is a local SQL instance."
	}
	return ""
}
