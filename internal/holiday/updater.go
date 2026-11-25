package holiday

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// maxDownloadSize 限制单次下载的大小，防止异常响应占用过多内存。
const maxDownloadSize = 1 << 20

// DefaultURL 返回某一年的数据在远端的地址。
func DefaultURL(year int) string {
	return fmt.Sprintf("https://cdn.jsdelivr.net/gh/NateScarlet/holiday-cn@master/%d.json", year)
}

// Updater 负责从远端拉取节假日数据，并把它保存在本地缓存目录里。
type Updater struct {
	Client *http.Client
	Dir    string                // 缓存目录
	URL    func(year int) string // 返回某一年的数据地址
}

// NewUpdater 返回一个把数据缓存到 dir 的 Updater，下载超时是 10 秒。
// 更新在后台进行，不挡界面，所以超时放宽了一些：实测这个数据源偶尔要 3 秒以上才有响应。
func NewUpdater(dir string) *Updater {
	return &Updater{
		Client: &http.Client{Timeout: 10 * time.Second},
		Dir:    dir,
		URL:    DefaultURL,
	}
}

// Fetch 下载并校验某一年的数据。
func (u *Updater) Fetch(ctx context.Context, year int) (YearData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.URL(year), nil)
	if err != nil {
		return YearData{}, err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return YearData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return YearData{}, fmt.Errorf("下载 %d 年的节假日数据失败：HTTP %d", year, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize))
	if err != nil {
		return YearData{}, err
	}

	var data YearData
	if err := json.Unmarshal(body, &data); err != nil {
		return YearData{}, fmt.Errorf("解析 %d 年的节假日数据：%w", year, err)
	}
	if data.Year != year {
		return YearData{}, fmt.Errorf("远端返回的是 %d 年的数据，而不是 %d 年", data.Year, year)
	}
	if err := Validate(data); err != nil {
		return YearData{}, err
	}
	return data, nil
}

// Cache 把某一年的数据写入本地缓存。
func (u *Updater) Cache(data YearData) error {
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(u.Dir, 0o755); err != nil {
		return err
	}
	path := u.cachePath(data.Year)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cachePath 返回某一年缓存文件的路径。
func (u *Updater) cachePath(year int) string {
	return filepath.Join(u.Dir, fmt.Sprintf("%d.json", year))
}

// LoadCache 把缓存目录里的数据加载到日历中。
// 缓存里的年份会覆盖内置数据；无法使用的缓存文件会被跳过。
func LoadCache(c *Calendar, dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var data YearData
		if err := json.Unmarshal(body, &data); err != nil {
			continue
		}
		_ = c.AddYear(data)
	}
	return nil
}
