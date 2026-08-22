package plugin

import (
	"fmt"
	"net/http"

	"dsc/jobs"
	"dsc/libs/vodka"
)

// /jobs 管理 API：定时任务的增删查启停（认证与 /plugins 一致）。

// handleJobsList 列出全部定时任务。
func (m *Manager) handleJobsList(c *vodka.Context) error {
	return c.JSON(vodka.Map{
		"status": "success",
		"jobs":   m.ListJobs(),
	})
}

// handleJobsAdd 新增定时任务（body: name/cron/prompt/enabled/max_iterations）。
func (m *Manager) handleJobsAdd(c *vodka.Context) error {
	var req struct {
		Name          string `json:"name"`
		Cron          string `json:"cron"`
		Prompt        string `json:"prompt"`
		Enabled       bool   `json:"enabled"`
		MaxIterations int    `json:"max_iterations"`
	}
	if err := c.Bind(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}
	j := &jobs.Job{
		Name: req.Name, Cron: req.Cron, Prompt: req.Prompt,
		Enabled: req.Enabled, MaxIterations: req.MaxIterations,
	}
	if err := m.AddJob(j); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "job": j})
}

// handleJobsRemove 删除定时任务（body: id）。
func (m *Manager) handleJobsRemove(c *vodka.Context) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.Bind(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}
	if err := m.RemoveJob(req.ID); err != nil {
		return vodka.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "id": req.ID})
}

// handleJobsEnable 启用/停用定时任务（body: id/enabled）。
func (m *Manager) handleJobsEnable(c *vodka.Context) error {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
	}
	if err := m.SetJobEnabled(req.ID, req.Enabled); err != nil {
		return vodka.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "id": req.ID, "enabled": req.Enabled})
}
