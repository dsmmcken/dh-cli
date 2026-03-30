"""dx.layer chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import int_col, double_col

t = new_table([
    int_col("X", list(range(10))),
    double_col("Y1", [float(x) ** 0.5 for x in range(10)]),
    double_col("Y2", [float(x) * 0.3 for x in range(10)]),
])

fig_line = dx.line(t, x="X", y="Y1")
fig_scatter = dx.scatter(t, x="X", y="Y2")

my_layer = dx.layer(fig_line, fig_scatter)

layer_widget = ui.panel(my_layer, title="Layered Chart")
