"""dx.line_polar chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("R",     [1.0, 2.0, 3.0, 4.0, 5.0, 3.0, 1.0]),
    double_col("Theta", [0.0, 51.4, 102.9, 154.3, 205.7, 257.1, 308.6]),
])

my_line_polar = dx.line_polar(t, r="R", theta="Theta", title="Polar Line")

line_polar_widget = ui.panel(my_line_polar, title="Polar Line")
