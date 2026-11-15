package main

import (
	"sync"

	"github.com/egoist/mygo/ui"
)

// 空态插图:一张纸 + 一个向下的箭头,表示"把东西放到这儿"。
//
// 不复用 flowbite 那批操作图标:它们是给目录条目配的(每一行一个动作),
// 而这里要表达的是"这个区域可以接收文件"。用 ins.front 之类会读成
// "插入到前面"甚至"返回"——插图的意思错了比没有更糟。
//
// 描边用 currentColor,跟随调用处给的文字色;线宽 1.6 是 24 视图里的值,
// 放大到 40 DIP 时约 2.7 物理像素(1.75 倍屏),不至于糊成一片。
var illustrationOnce sync.Once
var dropIllustration *ui.SVG

func dropIllus() *ui.SVG {
	illustrationOnce.Do(func() {
		dropIllustration = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48" fill="none">` +
			// 纸张轮廓,右下角折起
			`<path fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" d="M13 5h14l8 8v30H13z"/>` +
			`<path fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" d="M27 5v8h8"/>` +
			// 三条内容线,弱化
			`<path stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" opacity="0.55" d="M19 22h10M19 28h10M19 34h6"/>` +
			`</svg>`))
	})
	return dropIllustration
}
