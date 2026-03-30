"""dx.treemap chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col, int_col

t = new_table([
    string_col("Label",  ["Total", "A", "B", "A1", "A2", "B1", "B2"]),
    string_col("Parent", ["",      "Total", "Total", "A", "A", "B", "B"]),
    int_col("Value",     [0,       0,       0,       10,  20,  15,  25]),
])

my_treemap = dx.treemap(t, names="Label", parents="Parent", values="Value", title="Treemap")

treemap_widget = ui.panel(my_treemap, title="Treemap")
