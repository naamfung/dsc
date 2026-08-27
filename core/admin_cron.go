package core

import (
	"net/http"

	"dsc/cron"
	"dsc/libs/vodka"
)

// /cron 管理 API：定时任务的增删查启停（认证与 /plugins 一致）。

// handleCronList 列出全部定时任务。
func (m *Manager) handleCronList(c *vodka.Context) error {
	return c.JSON(vodka.Map{
		"status": "success",
		"jobs":   m.ListCronJobs(),
	})
}

// handleCronAdd 新增定时任务（body: name/cron/prompt/enabled/max_iterations）。
func (m *Manager) handleCronAdd(c *vodka.Context) error {
	var req struct {
		Name          string `json:"name"`
		Cron          string `json:"cron"`
		Prompt        string `json:"prompt"`
		Enabled       bool   `json:"enabled"`
		MaxIterations int    `json:"max_iterations"`
	}
	if err := bindBody(c, &req); err != nil {
		return err
	}
	j := &cron.Job{
		Name: req.Name, Cron: req.Cron, Prompt: req.Prompt,
		Enabled: req.Enabled, MaxIterations: req.MaxIterations,
	}
	if err := m.AddCronJob(j); err != nil {
		return vodka.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "job": j})
}

// handleCronRemove 删除定时任务（body: id）。
func (m *Manager) handleCronRemove(c *vodka.Context) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindBody(c, &req); err != nil {
		return err
	}
	if err := m.RemoveCronJob(req.ID); err != nil {
		return vodka.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "id": req.ID})
}

// handleCronEnable 启用/停用定时任务（body: id/enabled）。
func (m *Manager) handleCronEnable(c *vodka.Context) error {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := bindBody(c, &req); err != nil {
		return err
	}
	if err := m.SetCronJobEnabled(req.ID, req.Enabled); err != nil {
		return vodka.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(vodka.Map{"status": "success", "id": req.ID, "enabled": req.Enabled})
}
