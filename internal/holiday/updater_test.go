package holiday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// newTestUpdater 返回一个指向测试服务器的 Updater，以及它的缓存目录。
func newTestUpdater(t *testing.T, handler http.HandlerFunc) (*Updater, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	updater := NewUpdater(dir)
	updater.URL = func(year int) string {
		return fmt.Sprintf("%s/%d.json", server.URL, year)
	}
	return updater, dir
}

func TestDefaultURL(t *testing.T) {
	want := "https://cdn.jsdelivr.net/gh/NateScarlet/holiday-cn@master/2027.json"
	if got := DefaultURL(2027); got != want {
		t.Errorf("DefaultURL(2027) = %q，期望 %q", got, want)
	}
}

func TestFetch(t *testing.T) {
	valid := `{"year":2027,"days":[{"name":"元旦","date":"2027-01-01","isOffDay":true}]}`

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{name: "合法数据", status: http.StatusOK, body: valid},
		{name: "远端返回 404", status: http.StatusNotFound, body: "not found", wantErr: true},
		{name: "不是 JSON", status: http.StatusOK, body: "<html>", wantErr: true},
		{
			name:    "年份不符",
			status:  http.StatusOK,
			body:    `{"year":2026,"days":[{"name":"元旦","date":"2026-01-01","isOffDay":true}]}`,
			wantErr: true,
		},
		{
			name:    "日期不合法",
			status:  http.StatusOK,
			body:    `{"year":2027,"days":[{"name":"元旦","date":"2027-13-01","isOffDay":true}]}`,
			wantErr: true,
		},
		{name: "没有任何记录", status: http.StatusOK, body: `{"year":2027,"days":[]}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater, _ := newTestUpdater(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			data, err := updater.Fetch(context.Background(), 2027)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Fetch() 意外成功：%+v", data)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch() 意外返回错误：%v", err)
			}
			want := YearData{Year: 2027, Days: []Day{{Name: "元旦", Date: "2027-01-01", IsOffDay: true}}}
			if !reflect.DeepEqual(data, want) {
				t.Errorf("Fetch() = %+v，期望 %+v", data, want)
			}
		})
	}
}

func TestCacheWritesYearFile(t *testing.T) {
	updater, dir := newTestUpdater(t, func(http.ResponseWriter, *http.Request) {})
	data := YearData{Year: 2028, Days: []Day{{Name: "元旦", Date: "2028-01-01", IsOffDay: true}}}

	if err := updater.Cache(data); err != nil {
		t.Fatalf("Cache() 意外返回错误：%v", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, "2028.json"))
	if err != nil {
		t.Fatalf("读取缓存文件：%v", err)
	}
	var got YearData
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("解析缓存文件：%v", err)
	}
	if !reflect.DeepEqual(got, data) {
		t.Errorf("缓存文件的内容 = %+v，期望 %+v", got, data)
	}
}

func TestLoadCacheOverridesEmbedded(t *testing.T) {
	cal := loadEmbeddedCalendar(t)
	dir := t.TempDir()
	cached := YearData{Year: 2026, Days: []Day{{Name: "测试节日", Date: "2026-03-03", IsOffDay: true}}}
	body, err := json.Marshal(cached)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := LoadCache(cal, dir); err != nil {
		t.Fatalf("LoadCache() 意外返回错误：%v", err)
	}

	tests := []struct {
		name string
		date string
		want bool
	}{
		{name: "缓存里的放假日（周二）", date: "2026-03-03", want: false},
		{name: "缓存覆盖之后，原来的国庆节记录不在了", date: "2026-10-01", want: true},
		{name: "2027 年仍然来自内置数据", date: "2027-02-06", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.IsWorkday(mustDate(t, tt.date)); got != tt.want {
				t.Errorf("IsWorkday(%s) = %v，期望 %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestLoadCacheSkipsUnusableFiles(t *testing.T) {
	cal := loadEmbeddedCalendar(t)
	dir := t.TempDir()

	// 内容损坏的缓存文件，以及扩展名不对的文件，都应该被跳过。
	files := map[string]string{
		"2028.json": "{not json",
		"notes.txt": `{"year":2029,"days":[{"name":"元旦","date":"2029-01-01","isOffDay":true}]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := LoadCache(cal, dir); err != nil {
		t.Fatalf("LoadCache() 意外返回错误：%v", err)
	}
	for _, year := range []int{2028, 2029} {
		if cal.HasYear(year) {
			t.Errorf("不应该加载 %d 年的数据", year)
		}
	}
	if cal.IsWorkday(mustDate(t, "2026-10-01")) {
		t.Error("跳过无效文件之后，内置数据应该保持完好")
	}
}

func TestLoadCacheWithoutDirectory(t *testing.T) {
	cal := NewCalendar()
	if err := LoadCache(cal, filepath.Join(t.TempDir(), "holidays")); err != nil {
		t.Fatalf("缓存目录不存在时 LoadCache() 意外返回错误：%v", err)
	}
}

// TestEmbeddedMatchesRemote 把内置数据与远端数据源逐条比对。
// 它需要联网，所以只在设置了环境变量 COWTIME_VERIFY_REMOTE 之后运行。
func TestEmbeddedMatchesRemote(t *testing.T) {
	if os.Getenv("COWTIME_VERIFY_REMOTE") == "" {
		t.Skip("需要联网。设置 COWTIME_VERIFY_REMOTE=1 之后运行。")
	}

	for _, year := range []int{2026, 2027} {
		t.Run(fmt.Sprint(year), func(t *testing.T) {
			updater := NewUpdater(t.TempDir())
			remote, err := updater.Fetch(context.Background(), year)
			if err != nil {
				t.Fatalf("下载 %d 年的数据：%v", year, err)
			}

			body, err := dataFiles.ReadFile(fmt.Sprintf("data/%d.json", year))
			if err != nil {
				t.Fatalf("读取内置数据：%v", err)
			}
			var embedded YearData
			if err := json.Unmarshal(body, &embedded); err != nil {
				t.Fatalf("解析内置数据：%v", err)
			}

			if !reflect.DeepEqual(remote.Days, embedded.Days) {
				t.Errorf("%d 年的内置数据与远端数据不一致。\n远端：%+v\n内置：%+v", year, remote.Days, embedded.Days)
			}
		})
	}
}
