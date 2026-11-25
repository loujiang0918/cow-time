package holiday

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed data/*.json
var dataFiles embed.FS

// LoadEmbedded 把编译进程序的节假日数据加载到日历中。
func LoadEmbedded(c *Calendar) error {
	entries, err := fs.ReadDir(dataFiles, "data")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		body, err := dataFiles.ReadFile("data/" + entry.Name())
		if err != nil {
			return err
		}
		var data YearData
		if err := json.Unmarshal(body, &data); err != nil {
			return fmt.Errorf("解析内置数据 %s：%w", entry.Name(), err)
		}
		if err := c.AddYear(data); err != nil {
			return fmt.Errorf("内置数据 %s：%w", entry.Name(), err)
		}
	}
	return nil
}
