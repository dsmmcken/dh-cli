"""dx.indicator chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import double_col

t = new_table([
    double_col("Value", [75.0]),
    double_col("Reference", [50.0]),
])

my_indicator = dx.indicator(t, value="Value", reference="Reference", title="Indicator")

indicator_widget = ui.panel(my_indicator, title="Indicator")
