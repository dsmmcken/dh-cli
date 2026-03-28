"""
Classic deephaven.plot.figure.Figure API rendering tests.

Tests that figures created with the classic plotting API render properly
instead of showing as "unknown figure". Examples adapted from:
https://deephaven.io/core/docs/how-to-guides/plotting/api-plotting.md

Variables exported (all standalone Figure objects via .show()):
  plot_xy          - basic XY line plot
  plot_multi       - multiple series on shared axes
  plot_cat         - category bar chart
  plot_hist        - histogram with chart title
  plot_pie         - pie chart
  plot_subplots    - 2-row subplot layout
"""
from deephaven import new_table, empty_table
from deephaven.column import int_col, double_col, string_col
from deephaven.plot.figure import Figure

# ── XY Series (basic line plot) ──
xy_source = new_table([
    int_col("X", list(range(20))),
    double_col("Y", [float(x) ** 0.5 for x in range(20)]),
])

plot_xy = (
    Figure()
    .plot_xy(series_name="sqrt(x)", t=xy_source, x="X", y="Y")
    .show()
)

# ── Multiple series on shared axes ──
multi_source = empty_table(20).update(
    ["X = 0.1 * i", "Y1 = sin(X)", "Y2 = cos(X)"]
)

plot_multi = (
    Figure()
    .plot_xy(series_name="sin", t=multi_source, x="X", y="Y1")
    .plot_xy(series_name="cos", t=multi_source, x="X", y="Y2")
    .show()
)

# ── Category plot ──
cat_source = new_table([
    string_col("Categories", ["A", "B", "C"]),
    int_col("Values", [1, 3, 5]),
])

plot_cat = (
    Figure()
    .plot_cat(series_name="Categories", t=cat_source, category="Categories", y="Values")
    .show()
)

# ── Histogram ──
hist_source = new_table([int_col("Values", [1, 2, 2, 3, 3, 3, 4, 4, 5])])

plot_hist = (
    Figure()
    .plot_xy_hist(series_name="Histogram Values", t=hist_source, x="Values", nbins=5)
    .chart_title(title="Histogram of Values")
    .show()
)

# ── Pie chart ──
pie_source = new_table([
    string_col("region", ["NE", "SE", "SW", "NW"]),
    int_col("expenses", [100, 200, 150, 250]),
])

plot_pie = (
    Figure()
    .plot_pie(series_name="Expenses by Region", t=pie_source, category="region", y="expenses")
    .show()
)

# ── Subplots (2 rows, 1 col) ──
sub_source = empty_table(100).update(
    ["X = i", "Y = cos(0.1 * i)", "Z = sin(0.1 * i)"]
)

plot_subplots = (
    Figure(rows=2, cols=1)
    .new_chart(row=0, col=0)
    .plot_xy(series_name="Cosine", t=sub_source, x="X", y="Y")
    .new_chart(row=1, col=0)
    .plot_xy(series_name="Sine", t=sub_source, x="X", y="Z")
    .show()
)
