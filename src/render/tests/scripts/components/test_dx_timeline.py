"""dx.timeline chart render test."""
import deephaven.plot.express as dx
from deephaven import ui, new_table
from deephaven.column import string_col, long_col

# Use long_col for Instant-backed timestamps (nanos since epoch)
t = new_table([
    string_col("Task", ["Design", "Build", "Test"]),
    long_col("Start", [1700000000000000000, 1700100000000000000, 1700200000000000000]),
    long_col("End",   [1700100000000000000, 1700200000000000000, 1700300000000000000]),
])

# Cast to Instant for timeline
t = t.update([
    "Start = epochNanosToInstant(Start)",
    "End   = epochNanosToInstant(End)",
])

my_timeline = dx.timeline(t, x_start="Start", x_end="End", y="Task", title="Project Timeline")

timeline_widget = ui.panel(my_timeline, title="Timeline")
