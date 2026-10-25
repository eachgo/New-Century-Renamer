package main

import (
	"sync"

	"github.com/egoist/mygo/ui"
)

// flowbite catalog icons, parsed once (ui.MustParseSVG is not free).
var (
	iconMu    sync.Mutex
	iconCache = map[string]*ui.SVG{}
)

func opIcon(opid string) *ui.SVG {
	iconMu.Lock()
	defer iconMu.Unlock()
	if s, ok := iconCache[opid]; ok {
		return s
	}
	body, ok := flowbiteBodies[opid]
	if !ok {
		return nil
	}
	s := ui.MustParseSVG([]byte(body))
	iconCache[opid] = s
	return s
}
