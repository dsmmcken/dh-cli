# Render Classic deephaven.plot.figure.Figure Objects

## Problem

Classic Deephaven `Figure` objects (from `deephaven.plot.figure.Figure().plot_xy(...).show()`)
display as "unknown figure" in `dh render` output. Only `deephaven.plot.express` (dx) figures
are properly rendered because:

1. **Auto-discovery misses them**: `WidgetClient.discoverWidgets()` only lists
   `deephaven.ui.Element` and `deephaven.ui.Dashboard` — `Figure` type is excluded.
2. **No WidgetPlugin registered**: The plugin map has a `WidgetPlugin` for
   `deephaven.plot.express.DeephavenFigure` but not for `Figure`.
3. **FigureStub assumes plotly JSON**: The existing `fetchFigureData()` calls
   `getDataAsString()` and parses plotly JSON — classic figures don't have that method;
   they expose a `dh.plot.Figure` object with `.charts`, `.series`, `.title`.

## Classic Figure JSAPI Shape

When `connection.getObject({ name, type: 'Figure' })` resolves, the returned object is a
`dh.plot.Figure` instance:

```
Figure {
  title: string,
  charts: Chart[],
  subscribe(callback): Subscription,
  close(): void,
}

Chart {
  title: string,
  chartType: string,  // 'XY', 'CATEGORY', 'PIE', 'OHLC', 'TREEMAP'
  axes: Axis[],
  series: Series[],
}

Axis { label: string, type: string ('X'|'Y'), formatType: string }
Series { name: string, plotStyle: string ('LINE'|'BAR'|'SCATTER'|...) }
```

## Changes

### 1. `src/render/src/FigureStub.mjs`

- Add `CLASSIC_FIGURE_TYPE = 'Figure'`
- Add `normalizeChartType(chartType, plotStyle)` → maps JSAPI enums to short names
- Add `parseClassicFigure(figure)` → extracts title, charts, series, axes into same
  `{ title, traceType, traceCount, axes, traceList, columnMappings, layoutProps }` shape
  that `parseFigurePayload()` returns for dx figures
- Update `fetchFigureData(exportedObject)` to detect classic figures
  (`typeof widget.getDataAsString !== 'function'`) and call `parseClassicFigure()` instead
- Update `FigureWidgetPlugin` to also handle classic figures via same detection
- Add second plugin entry in `createFigureStubPluginMap()` for `'Figure'` type

### 2. `src/render/src/WidgetClient.mjs`

- Add `'Figure'` to `RENDERABLE_TYPES` in `discoverWidgets()` so standalone classic
  figures are auto-discovered (lowest priority after Dashboard and Element).

### 3. `src/render/tests/scripts/components/test_classic_figure.py` (new)

Test scripts from the Deephaven docs (using `new_table`/`empty_table` for self-contained
data). Examples: basic XY, multiple series, category, histogram, pie, subplots.

### 4. Snapshot file

`src/render/tests/snapshots/components/test_classic_figure.snap` — expected output.

## Test Plan

1. Build binary: `CGO_ENABLED=0 make build && cp dh ~/.local/bin/dh`
2. Rebuild rootfs + snapshot (render JS is embedded in binary)
3. Run: `DH_HOME=/workspace/.dh dh exec --vm -c '<test code>'`
4. Verify figure metadata appears instead of "unknown figure"
