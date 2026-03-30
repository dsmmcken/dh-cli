"""dx.scatter_ternary chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    double_col("A", [0.1, 0.2, 0.5, 0.3, 0.6]),
    double_col("B", [0.3, 0.5, 0.2, 0.4, 0.1]),
    double_col("C", [0.6, 0.3, 0.3, 0.3, 0.3]),
    string_col("Cat", ["X", "Y", "X", "Y", "X"]),
])

my_scatter_ternary = dx.scatter_ternary(t, a="A", b="B", c="C", title="Ternary Scatter")
my_scatter_ternary_by = dx.scatter_ternary(t, a="A", b="B", c="C", by="Cat", title="Ternary Scatter By Cat")

scatter_ternary_widget = ui.panel(
    ui.flex(my_scatter_ternary, my_scatter_ternary_by, direction="column"),
    title="Ternary Scatter Plots",
)
