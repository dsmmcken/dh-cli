"""dx.line_ternary chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("A", [0.1, 0.2, 0.5, 0.3, 0.6]),
    double_col("B", [0.3, 0.5, 0.2, 0.4, 0.1]),
    double_col("C", [0.6, 0.3, 0.3, 0.3, 0.3]),
])

my_line_ternary = dx.line_ternary(t, a="A", b="B", c="C", title="Ternary Line")

line_ternary_widget = ui.panel(my_line_ternary, title="Ternary Line")
