from deephaven import ui, new_table
from deephaven.column import string_col
import random
import string

# Generate 100 random item names
random.seed(42)
items = sorted([
    ''.join(random.choices(string.ascii_lowercase, k=6)) + f"_{i:03d}"
    for i in range(100)
])

items_table = new_table([string_col("Label", items)])

@ui.component
def test_picker_options():
    selected, set_selected = ui.use_state(None)

    return ui.flex(
        ui.picker(
            items_table,
            label="Random Items",
            selected_key=selected,
            on_change=set_selected,
        ),
        ui.text(f"Selected: {selected}"),
        direction="column", gap="size-100",
    )

picker_options_widget = test_picker_options()
