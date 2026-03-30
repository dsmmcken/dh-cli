"""dx.line_mapbox chart render test (deprecated, prefer line_map)."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    string_col("City", ["New York", "London", "Tokyo", "Sydney"]),
    double_col("Lat",  [40.7, 51.5, 35.7, -33.9]),
    double_col("Lon",  [-74.0, -0.1, 139.7, 151.2]),
])

my_line_mapbox = dx.line_mapbox(t, lat="Lat", lon="Lon", title="Mapbox Line")

line_mapbox_widget = ui.panel(my_line_mapbox, title="Mapbox Line")
