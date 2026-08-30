package main

import (
	"encoding/json"
	"testing"

	"dsc-sdk"
)

// TestReadSkillView read_skill 视图：技能名徽标 + SKILL.md 正文。
func TestReadSkillView(t *testing.T) {
	view, _ := readSkillView([]byte(`{"name":"code-audit"}`), "# Code Audit\n\n按此执行")
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "plain" || v.Title != "Skill" || v.Badge == nil || v.Badge.Text != "code-audit" || v.Badge.Tone != "teal" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Body != "# Code Audit\n\n按此执行" {
		t.Fatalf("body = %q", v.Body)
	}
	// 缺 name 参数回退
	if view, _ := readSkillView([]byte(`{}`), "x"); len(view) != 0 {
		t.Fatalf("缺 name 应返回空视图: %q", view)
	}
}

// TestInstallSkillView install_skill 视图：安装数量徽标 + installed 字段。
func TestInstallSkillView(t *testing.T) {
	view, _ := installSkillView(`{"ok":true,"installed":["flat-skill","lua-plugin-creator"],"installedDir":"C:\\skills\\installed","note":"..."}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "Skill" || v.Badge == nil || v.Badge.Text != "2 installed" || v.Badge.Tone != "green" {
		t.Fatalf("view 头 = %+v", v)
	}
	if len(v.Fields) != 3 || v.Fields[0].Value != "flat-skill, lua-plugin-creator" || v.Fields[0].Tone != "green" || v.Fields[1].Value != "C:\\skills\\installed" {
		t.Fatalf("fields = %+v", v.Fields)
	}
}

// TestUninstallSkillView uninstall_skill 视图：黄色徽标 + 技能名。
func TestUninstallSkillView(t *testing.T) {
	view, _ := uninstallSkillView(`{"ok":true,"uninstalled":"flat-skill","note":"技能已卸载"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "Skill" || v.Badge == nil || v.Badge.Text != "uninstalled" || v.Badge.Tone != "yellow" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Fields[0].Value != "flat-skill" || v.Fields[0].Tone != "yellow" || v.Fields[1].Value != "技能已卸载" {
		t.Fatalf("fields = %+v", v.Fields)
	}
}
