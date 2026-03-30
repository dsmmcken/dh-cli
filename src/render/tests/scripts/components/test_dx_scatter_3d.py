"""dx.scatter_3d chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    double_col("X", [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]),
    double_col("Y", [2.0, 3.0, 1.0, 5.0, 4.0, 6.0]),
    double_col("Z", [3.0, 1.0, 4.0, 2.0, 6.0, 5.0]),
    string_col("Cat", ["A", "B", "A", "B", "A", "B"]),
])

my_scatter_3d = dx.scatter_3d(t, x="X", y="Y", z="Z", title="3D Scatter")
my_scatter_3d_by = dx.scatter_3d(t, x="X", y="Y", z="Z", by="Cat", title="3D Scatter By Cat")

scatter_3d_widget = ui.panel(
    ui.flex(my_scatter_3d, my_scatter_3d_by, direction="column"),
    title="3D Scatter Plots",
)
