from deephaven import empty_table

t = empty_table(10).update([
    "X = i",
    "Y = X * 2",
    "Label = (X % 2 == 0) ? `even` : `odd`",
])
