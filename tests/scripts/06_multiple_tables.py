# Create multiple tables (use with --show-tables)
from deephaven import empty_table

numbers = empty_table(5).update(["Value = i + 1"])
squares = empty_table(5).update(["N = i + 1", "Square = (i + 1) * (i + 1)"])
fibonacci = empty_table(10).update([
    "N = i",
    "Fib = (long)(Math.round((Math.pow((1 + Math.sqrt(5)) / 2, i) - Math.pow((1 - Math.sqrt(5)) / 2, i)) / Math.sqrt(5)))"
])

print("Created 3 tables: numbers, squares, fibonacci")
