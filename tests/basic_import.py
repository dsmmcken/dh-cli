from deephaven import read_csv, ui

t = ui.table(read_csv("./tests/sample_data.csv"))

