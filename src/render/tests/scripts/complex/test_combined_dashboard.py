from deephaven import ui, empty_table

t = empty_table(10).update(["X = i", "Y = i * 2", "Label = (i % 2 == 0) ? `Even` : `Odd`"])

@ui.component
def dashboard():
    return ui.panel(
        ui.table(t),
        title="Data Dashboard"
    )

combined_dashboard_widget = dashboard()
