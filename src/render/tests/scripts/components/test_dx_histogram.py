"""dx.histogram chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col, string_col

t = new_table([
    double_col("Value", [1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0, 2.0, 3.0, 3.0]),
    string_col("Cat", ["A", "B"] * 6),
])

my_histogram = dx.histogram(t, x="Value", nbins=5, title="Histogram")
my_histogram_by = dx.histogram(t, x="Value", by="Cat", title="Histogram By Category")

histogram_widget = ui.panel(
    ui.flex(my_histogram, my_histogram_by, direction="column"),
    title="Histograms",
)
