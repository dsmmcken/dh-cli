"""dx.density_heatmap chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("X", [1.0, 2.0, 2.0, 3.0, 3.0, 3.0, 4.0, 4.0, 5.0, 1.5, 2.5, 3.5]),
    double_col("Y", [2.0, 3.0, 1.0, 5.0, 4.0, 2.0, 3.0, 1.0, 4.0, 3.0, 2.0, 5.0]),
])

my_density_heatmap = dx.density_heatmap(t, x="X", y="Y", title="Density Heatmap")

density_heatmap_widget = ui.panel(my_density_heatmap, title="Density Heatmap")
