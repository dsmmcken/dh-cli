"""dx.bar chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col, int_col

t = new_table([
    string_col("Category", ["A", "B", "C", "D"]),
    int_col("Value", [10, 25, 15, 30]),
    string_col("Group", ["X", "X", "Y", "Y"]),
])

my_bar = dx.bar(t, x="Category", y="Value", title="Simple Bar")
my_bar_by = dx.bar(t, x="Category", y="Value", by="Group", title="Bar By Group")

bar_widget = ui.panel(
    ui.flex(my_bar, my_bar_by, direction="column"),
    title="Bar Charts",
)
