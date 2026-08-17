package plugin

import (
	"fmt"
	"net/http"

	"dsc/libs/vodka"
)

// StartAdmin 啟動管理 API HTTP 服務（基於 Vodka 框架）
func (m *Manager) StartAdmin(addr string) {
	e := vodka.New()

	admin := e.Group("/plugins")
	admin.Post("/load", m.handleLoad)
	admin.Post("/unload", m.handleUnload)
	admin.Post("/reload", m.handleReload)
	admin.Get("/list", m.handleList)
	admin.Post("/metadata", m.handleMetadata)

	go func() {
		m.logger.Info("admin api listening", "addr", addr)
		e.Server.Addr = addr
		if err := e.Server.ListenAndServe(); err != nil {
			m.logger.Error("admin api error", "error", err)
		}
	}()
}

// handleLoad 處理加載插件請求
func (m *Manager) handleLoad(c *vodka.Context) error {
	var entry PluginEntry
	if err := c.Bind(&entry); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}

	if err := m.LoadPlugin(entry); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to load plugin: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "plugin loaded"})
}

// handleUnload 處理卸載插件請求
func (m *Manager) handleUnload(c *vodka.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}

	if err := m.UnloadPlugin(req.Name); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to unload plugin: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "plugin unloaded"})
}

// handleReload 處理重載插件請求
func (m *Manager) handleReload(c *vodka.Context) error {
	var entry PluginEntry
	if err := c.Bind(&entry); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}

	// 先卸載
	if err := m.UnloadPlugin(entry.Name); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to unload plugin: %v", err))
	}

	// 再加載
	if err := m.LoadPlugin(entry); err != nil {
		return vodka.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("failed to reload plugin: %v", err))
	}

	return c.JSON(vodka.Map{"status": "success", "message": "plugin reloaded"})
}

// handleList 處理列出插件請求
func (m *Manager) handleList(c *vodka.Context) error {
	plugins := m.ListPlugins()
	return c.JSON(vodka.Map{
		"status":  "success",
		"plugins": plugins,
	})
}

// handleMetadata 處理獲取插件元數據請求
func (m *Manager) handleMetadata(c *vodka.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}

	info, exists := m.GetPluginMetadata(req.Name)
	if !exists {
		return vodka.NewHTTPError(http.StatusNotFound, fmt.Sprintf("plugin '%s' not found", req.Name))
	}

	return c.JSON(vodka.Map{
		"status": "success",
		"metadata": map[string]string{
			"type":        info.Type,
			"name":        info.Name,
			"version":     info.Version,
			"api_version": info.ApiVersion,
		},
	})
}
