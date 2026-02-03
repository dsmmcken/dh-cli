# Simple data analysis example (use with --show-tables)
from deephaven import empty_table

# Create sample sales data
sales = empty_table(100).update([
    "Day = i + 1",
    "Region = (i % 4 == 0) ? `North` : ((i % 4 == 1) ? `South` : ((i % 4 == 2) ? `East` : `West`))",
    "Sales = Math.round(Math.random() * 1000 + 500)",
    "Quantity = (int)(Math.random() * 50 + 10)"
])

# Aggregate by region
by_region = sales.agg_by([
    agg.avg("AvgSales = Sales"),
    agg.sum_("TotalQuantity = Quantity"),
    agg.count_("Count")
], "Region")

print("Sales analysis complete")
