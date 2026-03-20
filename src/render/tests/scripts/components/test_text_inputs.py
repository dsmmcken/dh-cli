from deephaven import ui

@ui.component
def test_text_inputs():
    text_val, set_text_val = ui.use_state("hello")
    area_val, set_area_val = ui.use_state("multi\nline")
    search_val, set_search_val = ui.use_state("")
    num_val, set_num_val = ui.use_state(42)

    return ui.flex(
        ui.text_field(label="Text Field", value=text_val, on_change=set_text_val),
        ui.text(f"Text: {text_val}"),
        ui.text_area(label="Text Area", value=area_val, on_change=set_area_val),
        ui.text(f"Area: {area_val}"),
        ui.search_field(label="Search", value=search_val, on_change=set_search_val),
        ui.text(f"Search: {search_val}"),
        ui.number_field(label="Number", value=num_val, on_change=set_num_val, min_value=0, max_value=100),
        ui.text(f"Number: {num_val}"),
        direction="column", gap="size-100",
    )

text_inputs_widget = test_text_inputs()
