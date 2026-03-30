"""dx.line_3d chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("X", [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0]),
    double_col("Y", [2.0, 3.0, 1.0, 5.0, 4.0, 6.0, 3.0, 7.0]),
    double_col("Z", [3.0, 1.0, 4.0, 2.0, 6.0, 5.0, 2.0, 4.0]),
])

my_line_3d = dx.line_3d(t, x="X", y="Y", z="Z", title="3D Line")

line_3d_widget = ui.panel(my_line_3d, title="3D Line")
