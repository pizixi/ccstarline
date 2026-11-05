package main

// 所有样式直接写在本文件中，修改后重新编译即可。
// starship.toml 仅作为设计参考，程序不读取外部样式文件。
type Config struct {
	Directory DirectoryStyle
	GitBranch GitStyle
	Options   Options
	Colors    map[string]RGB
}

type DirectoryStyle struct {
	Length        int
	Symbol        string
	Substitutions map[string]string
}

type GitStyle struct {
	Symbol string
}

type Options struct {
	Modules           []string
	BarWidth          int
	NoSeparatorBefore []string // 这些模块与前一段之间不画箭头，直接换色
	GitTimeoutMS      int
	TimeLayout        string // Go 时间模板，例如 01-02 15:04。
}

func defaultConfig() Config {
	return Config{
		// 配色使用 0xRRGGBB；保留参考文件中的 ribbon 色带。
		Colors: map[string]RGB{
			"light":           rgb(0xfff6fb), // 浅色文字
			"dark":            rgb(0x252533), // 深色文字
			"user_bg":         rgb(0x9857a4), // 模型：紫色
			"directory_bg":    rgb(0xdb788c), // 目录 / 高占用警示：玫瑰粉
			"git_bg":          rgb(0xedb57c), // Git / 中占用警示：杏色
			"language_bg":     rgb(0x83bed2), // 上下文：浅蓝
			"tokens_bg":       rgb(0xa78bd0), // Token：薰衣草紫
			"context_low_bg":  rgb(0xa3be8c), // 上下文：低占用淡绿
			"context_mid_bg":  rgb(0xe6b07a), // 上下文：中占用淡橘
			"context_high_bg": rgb(0xd98b8b), // 上下文：高占用淡红
			"docker_bg":       rgb(0x41a6a0), // 额度与费用：青绿
			"time_bg":         rgb(0x83a7d8), // 时间：蓝色
		},
		Directory: DirectoryStyle{
			Length: 3, Symbol: "…/",
			Substitutions: map[string]string{
				"Documents": "󰈙", "Downloads": "", "Music": "", "Pictures": "",
			},
		},
		GitBranch: GitStyle{Symbol: ""},
		Options: Options{
			// 调整顺序或删除模块即可改变布局，也可加入 tokens、session。
			Modules:           []string{"model", "directory", "git", "context", "tokens", "usage", "cost", "time"},
			BarWidth:          8,
			NoSeparatorBefore: []string{"tokens"},
			GitTimeoutMS:      500,
			TimeLayout:        "01-02 15:04",
		},
	}
}

func (c Config) joined(module string) bool {
	for _, name := range c.Options.NoSeparatorBefore {
		if name == module {
			return true
		}
	}
	return false
}

func (c Config) hasModule(name string) bool {
	for _, item := range c.Options.Modules {
		if item == name {
			return true
		}
	}
	return false
}
