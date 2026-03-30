"""dx.funnel chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col, int_col

t = new_table([
    string_col("Stage", ["Visited", "Cart", "Checkout", "Purchased"]),
    int_col("Count", [1000, 600, 400, 200]),
])

my_funnel = dx.funnel(t, x="Count", y="Stage", title="Funnel Chart")

funnel_widget = ui.panel(my_funnel, title="Funnel Chart")
