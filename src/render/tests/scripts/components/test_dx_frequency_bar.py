"""dx.frequency_bar chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col

t = new_table([
    string_col("Color", ["Red", "Blue", "Red", "Green", "Blue", "Red", "Green", "Blue", "Blue"]),
    string_col("Size", ["S", "M", "L", "S", "M", "L", "S", "M", "L"]),
])

my_freq_bar = dx.frequency_bar(t, x="Color", title="Frequency Bar")
my_freq_bar_by = dx.frequency_bar(t, x="Color", by="Size", title="Frequency Bar By Size")

frequency_bar_widget = ui.panel(
    ui.flex(my_freq_bar, my_freq_bar_by, direction="column"),
    title="Frequency Bar Charts",
)
