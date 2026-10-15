// Play field geometry in internal/play coordinates (1280×720 screen, newLayout).
export const FIELD = { top: 104, line: 596, bottom: 650 };

// lanes mirrors internal/play newLayout: center lane 1.1, scratch 1.25, 12 px between groups.
// scale multiplies the lane unit only; x is relative to the field's left edge.
export function lanes(c, scale = 1) {
    const layout = c.layout || [], weights = [], gap = i => i > 0 && layout.length > 0 && layout[i].group !== layout[i - 1].group;
    let units = 0, gaps = 0;
    for (let i = 0; i < c.lanes; i++) {
        let w = 1;
        if (c.lanes % 2 === 1 && i === Math.floor(c.lanes / 2)) w = 1.1;
        if (layout[i]?.role === 'scratch') w = 1.25;
        weights.push(w); units += w; if (gap(i)) gaps += 12;
    }
    const unit = Math.min(64, (820 - gaps) / units) * scale, list = [];
    let x = 0;
    weights.forEach((w, i) => { if (gap(i)) x += 12; list.push({ x, width: unit * w }); x += unit * w; });
    return { width: x, lanes: list };
}
