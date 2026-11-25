package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontendUsesServiceFQN 保证前端写死的全名与 Go 侧的包路径一致。
// 挪动这个包之后这个测试会失败，提醒同步修改 frontend/main.js。
func TestFrontendUsesServiceFQN(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "frontend", "main.js"))
	if err != nil {
		t.Fatalf("读取 frontend/main.js：%v", err)
	}
	want := `const SERVICE = "` + FQN + `";`
	if !strings.Contains(string(body), want) {
		t.Errorf("frontend/main.js 里应该有这一行：%s", want)
	}
}
