from deephaven import ui

@ui.component
def counter():
    count, set_count = ui.use_state(0)
    text, set_text = ui.use_state("Hello")

    return ui.flex(
        ui.heading(f"Count: {count}"),
        ui.text(f"Message: {text}"),
        ui.button("Increment", on_press=lambda: set_count(count + 1)),
        ui.button("Reset", on_press=lambda: set_count(0)),
        ui.text_field(label="Message", value=text, on_change=set_text),
        direction="column",
        gap="size-200",
    )

counter_widget = counter()
