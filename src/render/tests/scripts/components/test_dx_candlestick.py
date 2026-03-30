"""dx.candlestick chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import int_col, double_col

t = new_table([
    int_col("Day", [1, 2, 3, 4, 5]),
    double_col("Open",  [100.0, 102.0, 101.0, 105.0, 103.0]),
    double_col("High",  [105.0, 106.0, 107.0, 108.0, 106.0]),
    double_col("Low",   [ 98.0,  99.0, 100.0, 102.0, 101.0]),
    double_col("Close", [103.0, 101.0, 106.0, 103.0, 105.0]),
])

my_candlestick = dx.candlestick(t, x="Day", open="Open", high="High", low="Low", close="Close")

candlestick_widget = ui.panel(my_candlestick, title="Candlestick Chart")
