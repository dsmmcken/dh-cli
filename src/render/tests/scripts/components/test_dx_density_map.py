"""dx.density_map chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("Lat", [40.7, 40.8, 40.6, 40.75, 40.65, 40.9, 40.5]),
    double_col("Lon", [-74.0, -73.9, -74.1, -73.95, -74.05, -73.85, -74.15]),
    double_col("Val", [10.0, 20.0, 15.0, 25.0, 30.0, 5.0, 12.0]),
])

my_density_map = dx.density_map(t, lat="Lat", lon="Lon", z="Val", title="Density Map")

density_map_widget = ui.panel(my_density_map, title="Density Map")
