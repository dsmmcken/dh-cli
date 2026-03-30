"""dx.area chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import int_col, double_col, string_col

t = new_table([
    int_col("X", list(range(20))),
    double_col("Y", [float(x) ** 0.5 for x in range(20)]),
    string_col("Cat", ["A", "B"] * 10),
])

my_area = dx.area(t, x="X", y="Y", title="Simple Area")
my_area_by = dx.area(t, x="X", y="Y", by="Cat", title="Area By Category")

area_widget = ui.panel(
    ui.flex(my_area, my_area_by, direction="column"),
    title="Area Charts",
)
