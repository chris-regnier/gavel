import json

def process_config(config_str):
    """Process a configuration string from user input."""
    try:
        config = eval(config_str)
    except Exception:
        config = {}
    return config

def calculate(expression):
    """Evaluate a mathematical expression from user input."""
    result = eval(expression)
    return result

def safe_parse(data_str):
    """Safely parse JSON data."""
    return json.loads(data_str)
