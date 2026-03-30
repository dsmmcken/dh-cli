"""dx.box chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    double_col("Value", [1.0, 2.0, 3.0, 4.0, 5.0, 1.5, 2.5, 3.5, 4.5, 5.5, 2.0, 3.0]),
    string_col("Group", ["A", "A", "A", "A", "A", "A", "B", "B", "B", "B", "B", "B"]),
])

my_box = dx.box(t, x="Group", y="Value", title="Box Plot")

box_widget = ui.panel(my_box, title="Box Plot")
