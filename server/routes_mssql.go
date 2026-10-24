package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"docklog/config"
	"docklog/db"
	"docklog/middleware"
	"docklog/models"
	appmssql "docklog/mssql"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func (s *Server) registerMSSQLRoutes(r *echo.Group) {
	r.GET("/mssql/status", func(c echo.Context) error {
		cfg, err := db.GetMSSQLSettings()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"configured": db.MSSQLConfigured(),
			"enabled":    cfg.Enabled,
			"host":       cfg.Host,
			"database":   cfg.DatabaseName,
			"table":      cfg.TableName,
			"schema":     "container_logs.dbo.container_logs",
		})
	})

	r.GET("/mssql/containers", s.handleMSSQLContainers)
	r.GET("/mssql/logs", s.handleMSSQLLogs)
	r.GET("/mssql/logs/count", s.handleMSSQLLogsCount)
	r.GET("/mssql/logs/download", s.handleMSSQLLogsDownload)

	admin := r.Group("/admin")
	admin.Use(s.adminMiddleware())
	admin.GET("/mssql-settings", s.handleGetMSSQLSettings)
	admin.PUT("/mssql-settings", s.handlePutMSSQLSettings)
	admin.POST("/mssql-settings/test", s.handleTestMSSQLSettings)
}

func (s *Server) registerMSSQLWebSocketRoutes() {
	s.echo.GET("/ws/mssql/logs", s.handleMSSQLLogsWebSocket)
}

func (s *Server) handleGetMSSQLSettings(c echo.Context) error {
	cfg, err := db.GetMSSQLSettings()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	cfg.Password = ""
	return c.JSON(http.StatusOK, cfg)
}

type mssqlSettingsPayload struct {
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	Username               string `json:"username"`
	Password               string `json:"password"`
	DatabaseName           string `json:"database"`
	TableName              string `json:"table"`
	IDColumn               string `json:"id_column"`
	TimestampColumn        string `json:"timestamp_column"`
	MessageColumn          string `json:"message_column"`
	Enabled                bool   `json:"enabled"`
	TrustServerCertificate *bool  `json:"trust_server_certificate"`
	EncryptDisable         bool   `json:"encrypt_disable"`
}

func mssqlTrustFromPayload(trust *bool, encryptDisable bool) bool {
	if encryptDisable {
		return false
	}
	if trust == nil {
		return true
	}
	return *trust
}

func (s *Server) handlePutMSSQLSettings(c echo.Context) error {
	var payload mssqlSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	in := db.MSSQLSettings{
		Host:                   strings.TrimSpace(payload.Host),
		Port:                   payload.Port,
		Username:               strings.TrimSpace(payload.Username),
		DatabaseName:           strings.TrimSpace(payload.DatabaseName),
		TableName:              strings.TrimSpace(payload.TableName),
		IDColumn:               strings.TrimSpace(payload.IDColumn),
		TimestampColumn:        strings.TrimSpace(payload.TimestampColumn),
		MessageColumn:          strings.TrimSpace(payload.MessageColumn),
		Enabled:                payload.Enabled,
		TrustServerCertificate: mssqlTrustFromPayload(payload.TrustServerCertificate, payload.EncryptDisable),
		EncryptDisable:         payload.EncryptDisable,
	}
	db.ApplyMSSQLDefaults(&in)
	if err := db.SaveMSSQLSettings(in, strings.TrimSpace(payload.Password)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if token, ok := c.Get("user").(*jwt.Token); ok {
		if claims, ok := token.Claims.(*models.UserClaims); ok {
			s.audit(claims.ID, s.auditActor(claims), "update_mssql_settings", "mssql", "success", "Updated MS SQL Server log source settings")
		}
	}
	cfg, _ := db.GetMSSQLSettings()
	cfg.Password = ""
	return c.JSON(http.StatusOK, cfg)
}

func (s *Server) handleTestMSSQLSettings(c echo.Context) error {
	var payload mssqlSettingsPayload
	if err := c.Bind(&payload); err == nil && strings.TrimSpace(payload.Host) != "" {
		in := db.MSSQLSettings{
			Host:                   strings.TrimSpace(payload.Host),
			Port:                   payload.Port,
			Username:               strings.TrimSpace(payload.Username),
			Password:               strings.TrimSpace(payload.Password),
			DatabaseName:           strings.TrimSpace(payload.DatabaseName),
			TableName:              strings.TrimSpace(payload.TableName),
			IDColumn:               strings.TrimSpace(payload.IDColumn),
			TimestampColumn:        strings.TrimSpace(payload.TimestampColumn),
			MessageColumn:          strings.TrimSpace(payload.MessageColumn),
			Enabled:                true,
			TrustServerCertificate: mssqlTrustFromPayload(payload.TrustServerCertificate, payload.EncryptDisable),
			EncryptDisable:         payload.EncryptDisable,
		}
		db.ApplyMSSQLDefaults(&in)
		if in.Password == "" {
			stored, err := db.GetMSSQLSettingsWithPassword()
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			in.Password = stored.Password
		}
		ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
		defer cancel()
		conn, err := appmssql.OpenDB(ctx, in)
		if err != nil {
			msg := err.Error() + appmssql.ConnectionHint(err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
		}
		conn.Close()
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()
	if err := appmssql.TestConnection(ctx); err != nil {
		msg := err.Error() + appmssql.ConnectionHint(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func mssqlLogFilter(c echo.Context) appmssql.LogFilter {
	return appmssql.LogFilterFromQuery(c.QueryParam("container"))
}

func (s *Server) handleMSSQLContainers(c echo.Context) error {
	if !db.MSSQLConfigured() {
		return c.JSON(http.StatusOK, []string{})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()
	names, err := appmssql.ListContainers(ctx, 500)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, names)
}

func (s *Server) handleMSSQLLogs(c echo.Context) error {
	if !db.MSSQLConfigured() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "MS SQL log source is not configured"})
	}
	limit := appmssql.ClampBatchLimit(0)
	if v, err := strconv.Atoi(c.QueryParam("tail")); err == nil && v > 0 {
		limit = appmssql.ClampBatchLimit(v)
	}
	filter := mssqlLogFilter(c)
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Minute)
	defer cancel()

	if strings.EqualFold(c.QueryParam("all"), "true") || c.QueryParam("all") == "1" {
		ctxAll, cancelAll := context.WithTimeout(c.Request().Context(), 10*time.Minute)
		defer cancelAll()
		total, err := appmssql.CountLogs(ctxAll, filter)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		rows, err := appmssql.FetchAll(ctxAll, filter)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		n := len(rows)
		pageSize := n
		if pageSize <= 0 {
			pageSize = 1
		}
		return c.JSON(http.StatusOK, appmssql.LogsPayloadPage(rows, 1, pageSize, total))
	}

	total, err := appmssql.CountLogs(ctx, filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	beforeID := int64(0)
	beforeIDStr := strings.TrimSpace(c.QueryParam("before_id"))
	if beforeIDStr != "" {
		parsed, err := strconv.ParseInt(beforeIDStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid before_id"})
		}
		beforeID = parsed
	}

	rows, err := appmssql.FetchBatch(ctx, limit, beforeID, filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, appmssql.LogsPayloadBatch(rows, limit, total))
}

func (s *Server) handleMSSQLLogsCount(c echo.Context) error {
	if !db.MSSQLConfigured() {
		return c.JSON(http.StatusOK, map[string]int64{"count": 0})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()
	count, err := appmssql.CountLogs(ctx, mssqlLogFilter(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]int64{"count": count})
}

func (s *Server) handleMSSQLLogsDownload(c echo.Context) error {
	if !db.MSSQLConfigured() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "MS SQL log source is not configured"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Minute)
	defer cancel()
	rows, err := appmssql.FetchAll(ctx, mssqlLogFilter(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	body := strings.Join(appmssql.RowsToLines(rows), "\n")
	if body != "" {
		body += "\n"
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="mssql-logs.log"`)
	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

func (s *Server) handleMSSQLLogsWebSocket(c echo.Context) error {
	if !configMSSQLConfigured() {
		return c.NoContent(http.StatusServiceUnavailable)
	}

	if !config.AuthDisabled {
		if _, err := middleware.AuthenticateWS(c); err != nil {
			return middleware.WSAuthError(c, err)
		}
	}

	ws, err := middleware.UpgradeAuthenticatedWS(c)
	if err != nil {
		return err
	}
	defer ws.Close()

	filter := mssqlLogFilter(c)
	lastID := int64(0)
	if v := strings.TrimSpace(c.QueryParam("after_id")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastID = parsed
		}
	}

	ctx, cancel := context.WithCancel(c.Request().Context())
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			qCtx, qCancel := context.WithTimeout(ctx, 10*time.Second)
			rows, err := appmssql.FetchAfterID(qCtx, lastID, 500, filter)
			qCancel()
			if err != nil {
				msg := "[DockLog] MS SQL poll error: " + err.Error() + "\n"
				_ = ws.WriteMessage(websocket.TextMessage, []byte(msg))
				continue
			}
			for _, row := range rows {
				if row.ID > lastID {
					lastID = row.ID
				}
				payload, err := json.Marshal(appmssql.RowToEntry(row))
				if err != nil {
					continue
				}
				payload = append(payload, '\n')
				if err := ws.WriteMessage(websocket.TextMessage, payload); err != nil {
					return nil
				}
			}
		}
	}
}

func configMSSQLConfigured() bool {
	return db.MSSQLConfigured()
}
