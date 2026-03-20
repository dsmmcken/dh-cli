"""
Simple scatter plot widget for figure investigation.

Creates both a bare figure (for direct access) and a ui.panel wrapping it
(for embedded-in-widget access).

Variables exported:
  my_scatter       - bare DeephavenFigure (dx.scatter)
  my_hist          - bare DeephavenFigure (dx.histogram)
  scatter_widget   - ui.panel containing the scatter plot
  combo_widget     - ui.panel containing scatter + histogram + table
"""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import int_col, double_col, string_col

t = new_table([
    int_col("X", list(range(20))),
    double_col("Y", [x**0.5 for x in range(20)]),
    string_col("Category", ["alpha", "beta"] * 10),
])

# Bare figures (accessible directly by name)
my_scatter = dx.scatter(t, x="X", y="Y", color="Category", title="Square Root")
my_hist = dx.histogram(t, x="Y", title="Y Distribution")

# Simple widget: just a figure in a panel
scatter_widget = ui.panel(my_scatter, title="Scatter Panel")

# Combo widget: figure + table side by side
combo_widget = ui.panel(
    ui.flex(
        my_scatter,
        my_hist,
        ui.table(t),
        direction="column",
    ),
    title="Combo Panel",
)
