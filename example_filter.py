import sys
import json


def main():
    input_data = sys.stdin.read()
    try:
        data = json.loads(input_data)
    except json.JSONDecodeError as e:
        print(f"Error decoding JSON: {e}")
        sys.exit(1)

    # print('Received data:')
    # print(data)
    # Check for the key-value pair
    if 'name' in data and data['name'].startswith('MAD'):
        sys.exit(0)
    else:
        sys.exit(1)


if __name__ == "__main__":
    main()
