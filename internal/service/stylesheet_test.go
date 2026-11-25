package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 透明度靠样式变量 --card-alpha 生效：main.js 把它写在 body 上，样式表在卡片上读取它。
// 卡片如果自己再声明一次这个变量，就会挡住从 body 继承来的值，滑块会完全失效。
// 这条测试守住这个约定。
func TestCardAlphaIsSetOnBodyAndNeverDeclaredOnTheCard(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("..", "..", "frontend", "style.css"))
	if err != nil {
		t.Fatalf("读取 frontend/style.css：%v", err)
	}
	if regexp.MustCompile(`(?m)^\s*--card-alpha\s*:`).Match(css) {
		t.Error("style.css 不应该声明 --card-alpha，否则会挡住 main.js 写在 body 上的值；初始值只应写在 var() 的回退位置")
	}
	if !strings.Contains(string(css), "var(--card-alpha") {
		t.Error("style.css 应该通过 var(--card-alpha) 读取底色的不透明程度")
	}

	js, err := os.ReadFile(filepath.Join("..", "..", "frontend", "main.js"))
	if err != nil {
		t.Fatalf("读取 frontend/main.js：%v", err)
	}
	if !strings.Contains(string(js), `document.body.style.setProperty("--card-alpha"`) {
		t.Error("main.js 应该把 --card-alpha 写在 document.body 上")
	}
}
