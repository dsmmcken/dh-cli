"""dx.scatter_polar chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    double_col("R",     [1.0, 2.0, 3.0, 4.0, 5.0, 3.0]),
    double_col("Theta", [0.0, 60.0, 120.0, 180.0, 240.0, 300.0]),
    string_col("Cat",   ["A", "B", "A", "B", "A", "B"]),
])

my_scatter_polar = dx.scatter_polar(t, r="R", theta="Theta", title="Polar Scatter")
my_scatter_polar_by = dx.scatter_polar(t, r="R", theta="Theta", by="Cat", title="Polar Scatter By Cat")

scatter_polar_widget = ui.panel(
    ui.flex(my_scatter_polar, my_scatter_polar_by, direction="column"),
    title="Polar Scatter Plots",
)
