#!/usr/bin/python3
from grafanalib._gen import generate_dashboard

if __name__ == '__main__':
    generate_dashboard(args=['-o', 'main.json', 'poseidon.dashboard.py'])
