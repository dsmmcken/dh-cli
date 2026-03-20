from deephaven import ui

@ui.component
def test_accordion():
    return ui.flex(
        ui.accordion(
            ui.disclosure(title="Section 1", panel="Content of section 1"),
            ui.disclosure(title="Section 2", panel="Content of section 2"),
            ui.disclosure(title="Section 3", panel="Content of section 3"),
        ),
        direction="column", gap="size-100",
    )

accordion_widget = test_accordion()
