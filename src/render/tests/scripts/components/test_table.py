from deephaven import ui, empty_table

@ui.component
def test_table():
    t = empty_table(50).update(["X = i", "Y = Math.sin(i / 5.0)", "Label = (i % 2 == 0) ? `even` : `odd`"])
    return ui.flex(
        ui.heading("Table Component"),
        ui.table(t),
        direction="column", gap="size-100",
    )

table_widget = test_table()
