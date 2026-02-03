# Create a simple table (use with --show-tables)
from deephaven import empty_table

my_table = empty_table(10).update([
    "X = i",
    "Y = i * 2",
    "Z = Math.sqrt(i)"
])

print(f"Created table with {my_table.size} rows")
