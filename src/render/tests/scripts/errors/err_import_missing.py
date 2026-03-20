from deephaven import ui

@ui.component
def err_import_missing():
    # Bug: importing a module that doesn't exist, but inside the component
    # so it only fails at render time
    import nonexistent_plotting_library as npl
    chart = npl.create_chart([1, 2, 3])
    return ui.text(f"Chart: {chart}")

err_import_missing_widget = err_import_missing()
