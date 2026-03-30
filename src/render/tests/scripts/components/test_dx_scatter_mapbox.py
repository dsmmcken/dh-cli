"""dx.scatter_mapbox chart render test (deprecated, prefer scatter_map)."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col, int_col

t = new_table([
    string_col("City", ["New York", "London", "Tokyo", "Sydney"]),
    double_col("Lat",  [40.7, 51.5, 35.7, -33.9]),
    double_col("Lon",  [-74.0, -0.1, 139.7, 151.2]),
    int_col("Pop",     [8000, 9000, 14000, 5000]),
])

my_scatter_mapbox = dx.scatter_mapbox(t, lat="Lat", lon="Lon", size="Pop", title="Mapbox Scatter")

scatter_mapbox_widget = ui.panel(my_scatter_mapbox, title="Mapbox Scatter")
