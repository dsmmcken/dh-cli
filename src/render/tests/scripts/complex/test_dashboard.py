from deephaven import ui, empty_table

# Simple test component
t = empty_table(10).update(["X = i", "Y = i * 2"])

@ui.component
def my_dashboard():
    return ui.panel(
        ui.table(t),
        title="Test Dashboard"
    )

my_widget = my_dashboard()
