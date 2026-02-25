# Create a time-based table (use with --show-tables)
from deephaven import time_table
from deephaven.time import to_j_instant

# Create a ticking table (1 row per second)
# Note: This creates a live table, best viewed in Deephaven UI
ticks = time_table("PT1S").update([
    "Counter = ii",
    "RandomValue = Math.random()"
]).head(5)  # Limit to 5 rows for preview

print("Created time table with 5 rows")
