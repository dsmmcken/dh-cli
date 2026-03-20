from deephaven import ui

@ui.component
def test_layout():
    return ui.flex(
        ui.view(
            ui.heading("Flex Row"),
            ui.flex(
                ui.text("Item 1"),
                ui.text("Item 2"),
                ui.text("Item 3"),
                direction="row", gap="size-200",
            ),
            background_color="gray-100",
            padding="size-200",
        ),
        ui.divider(),
        ui.view(
            ui.heading("Grid Layout"),
            ui.grid(
                ui.text("A"), ui.text("B"),
                ui.text("C"), ui.text("D"),
                columns=["1fr", "1fr"],
                rows="auto",
                gap="size-100",
            ),
            background_color="gray-100",
            padding="size-200",
        ),
        direction="column", gap="size-200",
    )

layout_widget = test_layout()
