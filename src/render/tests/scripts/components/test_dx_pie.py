"""dx.pie chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col, int_col

t = new_table([
    string_col("Fruit", ["Apple", "Banana", "Cherry", "Date"]),
    int_col("Count", [40, 30, 20, 10]),
])

my_pie = dx.pie(t, names="Fruit", values="Count", title="Fruit Pie")

pie_widget = ui.panel(my_pie, title="Pie Chart")
