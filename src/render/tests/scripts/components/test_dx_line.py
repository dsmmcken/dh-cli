"""dx.line chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import int_col, double_col, string_col

t = new_table([
    int_col("X", list(range(20))),
    double_col("Y1", [float(x) ** 0.5 for x in range(20)]),
    double_col("Y2", [float(x) ** 0.3 for x in range(20)]),
    string_col("Cat", ["A", "B"] * 10),
])

my_line = dx.line(t, x="X", y="Y1", title="Simple Line")
my_line_by = dx.line(t, x="X", y="Y1", by="Cat", title="Line By Category")

line_widget = ui.panel(
    ui.flex(my_line, my_line_by, direction="column"),
    title="Line Charts",
)
