package play

import "ggosu/internal/settings"

func (g *scene) initView() error {
	keys := make([]string, len(g.bindings))
	for i, b := range g.bindings {
		keys[i] = b.name
	}
	var err error
	mode := g.mode
	if g.mods.Kind != "" {
		mode += " / " + g.mods.Kind
	}
	if g.mods.Visibility != "" {
		mode += " / " + g.mods.Visibility
	}
	g.view, err = newView(g.chart, g.session, g.images, viewConfig{Visibility: g.mods.Visibility, Keys: keys, Mode: mode, Offset: g.offsets.Total(), Judge: g.offsets.Judge(), Font: sceneFont(), Profile: &g.profile, Display: &g.display, Online: g.networkEnabled(), Debug: g.debug})
	if err == nil {
		g.view.editing = g.editing
		g.view.Update(viewState{Position: -g.preroll, Rate: g.speed, Scroll: g.scroll * settings.PixelsPerScroll})
	}
	return err
}
