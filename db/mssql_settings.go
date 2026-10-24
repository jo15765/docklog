package db

import (
	"database/sql"
	"errors"
	"time"
)

const MSSQLSettingsRowID = 1

const (
	DefaultMSSQLDatabase        = "container_logs"
	DefaultMSSQLTable           = "dbo.container_logs"
	DefaultMSSQLIDColumn        = "id"
	DefaultMSSQLTimestampColumn = "logged_at"
	DefaultMSSQLMessageColumn   = "message"
)

type MSSQLSettings struct {
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	Username               string `json:"username"`
	Password               string `json:"password,omitempty"`
	DatabaseName           string `json:"database"`
	TableName              string `json:"table"`
	IDColumn               string `json:"id_column"`
	TimestampColumn        string `json:"timestamp_column"`
	MessageColumn          string `json:"message_column"`
	Enabled                bool   `json:"enabled"`
	TrustServerCertificate bool   `json:"trust_server_certificate"`
	EncryptDisable         bool   `json:"encrypt_disable"`
	PasswordSet            bool   `json:"password_set"`
	UpdatedAt              string `json:"updated_at,omitempty"`
}

func ApplyMSSQLDefaults(s *MSSQLSettings) {
	if s == nil {
		return
	}
	if s.Port <= 0 {
		s.Port = 1433
	}
	if s.DatabaseName == "" {
		s.DatabaseName = DefaultMSSQLDatabase
	}
	if s.TableName == "" {
		s.TableName = DefaultMSSQLTable
	}
	if s.IDColumn == "" {
		s.IDColumn = DefaultMSSQLIDColumn
	}
	if s.TimestampColumn == "" {
		s.TimestampColumn = DefaultMSSQLTimestampColumn
	}
	if s.MessageColumn == "" {
		s.MessageColumn = DefaultMSSQLMessageColumn
	}
}

func migrateMSSQLSettingsSchema() error {
	_, err := DB.Exec(`
	CREATE TABLE IF NOT EXISTS mssql_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		host TEXT NOT NULL DEFAULT '',
		port INTEGER NOT NULL DEFAULT 1433,
		username TEXT NOT NULL DEFAULT '',
		password TEXT NOT NULL DEFAULT '',
		database_name TEXT NOT NULL DEFAULT 'container_logs',
		table_name TEXT NOT NULL DEFAULT 'dbo.container_logs',
		id_column TEXT NOT NULL DEFAULT 'id',
		timestamp_column TEXT NOT NULL DEFAULT 'logged_at',
		message_column TEXT NOT NULL DEFAULT 'message',
		enabled BOOLEAN NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO mssql_settings (id) VALUES (1);
	`)
	if err != nil {
		return err
	}
	_, _ = DB.Exec(`
		UPDATE mssql_settings SET
			database_name = 'container_logs'
		WHERE id = 1 AND (database_name = '' OR database_name = 'container_logs');
	`)
	_, _ = DB.Exec(`
		UPDATE mssql_settings SET
			table_name = 'dbo.container_logs',
			id_column = 'id',
			timestamp_column = 'logged_at',
			message_column = 'message'
		WHERE id = 1 AND (
			table_name = '' OR table_name = 'dbo.container_logs'
			OR id_column IN ('', 'Id') OR timestamp_column IN ('', 'Timestamp')
			OR message_column IN ('', 'Message')
		);
	`)
	for _, col := range []struct{ name, def string }{
		{"trust_server_certificate", "BOOLEAN NOT NULL DEFAULT 1"},
		{"encrypt_disable", "BOOLEAN NOT NULL DEFAULT 0"},
	} {
		if tableHasColumn("mssql_settings", col.name) {
			continue
		}
		_, _ = DB.Exec("ALTER TABLE mssql_settings ADD COLUMN " + col.name + " " + col.def)
	}
	return nil
}

func GetMSSQLSettings() (MSSQLSettings, error) {
	var s MSSQLSettings
	var password string
	var updatedAt sql.NullString
	var trust sql.NullBool
	var encryptOff sql.NullBool
	err := DB.QueryRow(`
		SELECT host, port, username, password, database_name, table_name,
		       id_column, timestamp_column, message_column, enabled,
		       trust_server_certificate, encrypt_disable, updated_at
		FROM mssql_settings WHERE id = ?`, MSSQLSettingsRowID).Scan(
		&s.Host, &s.Port, &s.Username, &password, &s.DatabaseName, &s.TableName,
		&s.IDColumn, &s.TimestampColumn, &s.MessageColumn, &s.Enabled,
		&trust, &encryptOff, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s = MSSQLSettings{Port: 1433, TrustServerCertificate: true}
			ApplyMSSQLDefaults(&s)
			return s, nil
		}
		return s, err
	}
	if !trust.Valid || trust.Bool {
		s.TrustServerCertificate = true
	} else {
		s.TrustServerCertificate = false
	}
	s.EncryptDisable = encryptOff.Valid && encryptOff.Bool
	s.PasswordSet = password != ""
	if updatedAt.Valid {
		s.UpdatedAt = updatedAt.String
	}
	ApplyMSSQLDefaults(&s)
	return s, nil
}

func GetMSSQLSettingsWithPassword() (MSSQLSettings, error) {
	s, err := GetMSSQLSettings()
	if err != nil {
		return s, err
	}
	var password string
	err = DB.QueryRow(`SELECT password FROM mssql_settings WHERE id = ?`, MSSQLSettingsRowID).Scan(&password)
	if err != nil {
		return s, err
	}
	s.Password = password
	s.PasswordSet = password != ""
	return s, nil
}

func SaveMSSQLSettings(in MSSQLSettings, newPassword string) error {
	current, err := GetMSSQLSettingsWithPassword()
	if err != nil {
		return err
	}
	password := current.Password
	if newPassword != "" {
		password = newPassword
	}
	ApplyMSSQLDefaults(&in)
	_, err = DB.Exec(`
		UPDATE mssql_settings SET
			host = ?, port = ?, username = ?, password = ?,
			database_name = ?, table_name = ?,
			id_column = ?, timestamp_column = ?, message_column = ?,
			enabled = ?, trust_server_certificate = ?, encrypt_disable = ?,
			updated_at = ?
		WHERE id = ?`,
		in.Host, in.Port, in.Username, password,
		in.DatabaseName, in.TableName,
		in.IDColumn, in.TimestampColumn, in.MessageColumn,
		in.Enabled, in.TrustServerCertificate, in.EncryptDisable,
		time.Now().UTC().Format(time.RFC3339),
		MSSQLSettingsRowID,
	)
	return err
}

func MSSQLConfigured() bool {
	s, err := GetMSSQLSettingsWithPassword()
	if err != nil || !s.Enabled {
		return false
	}
	ApplyMSSQLDefaults(&s)
	return s.Host != "" && s.Username != "" && s.Password != ""
}
