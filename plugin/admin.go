package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// StartAdmin 啟動管理 API HTTP 服務
func (m *Manager) StartAdmin(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/plugins/load", m.handleLoad)
	mux.HandleFunc("/plugins/unload", m.handleUnload)
	mux.HandleFunc("/plugins/reload", m.handleReload)
	mux.HandleFunc("/plugins/list", m.handleList)
	mux.HandleFunc("/plugins/metadata", m.handleMetadata)

	go func() {
		m.logger.Info("admin api listening", "addr", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			m.logger.Error("admin api error", "error", err)
		}
	}()
}

// handleLoad 處理加載插件請求
func (m *Manager) handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry PluginEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	if err := m.loadPluginFromEntryLocked(entry); err != nil {
		http.Error(w, fmt.Sprintf("failed to load plugin: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "plugin loaded"})
}

// handleUnload 處理卸載插件請求
func (m *Manager) handleUnload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	if err := m.UnloadPlugin(req.Name); err != nil {
		http.Error(w, fmt.Sprintf("failed to unload plugin: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "plugin unloaded"})
}

// handleReload 處理重載插件請求
func (m *Manager) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry PluginEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	// 先卸載
	if err := m.UnloadPlugin(entry.Name); err != nil {
		http.Error(w, fmt.Sprintf("failed to unload plugin: %v", err), http.StatusInternalServerError)
		return
	}

	// 再加載
	if err := m.loadPluginFromEntryLocked(entry); err != nil {
		http.Error(w, fmt.Sprintf("failed to reload plugin: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "plugin reloaded"})
}

// handleList 處理列出插件請求
func (m *Manager) handleList(w http.ResponseWriter, r *http.Request) {
	plugins := m.ListPlugins()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"plugins": plugins,
	})
}

// handleMetadata 處理獲取插件元數據請求
func (m *Manager) handleMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	info, exists := m.GetPluginMetadata(req.Name)
	if !exists {
		http.Error(w, fmt.Sprintf("plugin '%s' not found", req.Name), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"metadata": map[string]string{
			"type":    info.Type,
			"name":    info.Name,
			"version": info.Version,
			"api_version": info.ApiVersion,
		},
	})
}
