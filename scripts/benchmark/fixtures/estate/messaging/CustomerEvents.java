class CustomerEvents {
  void publishCustomerChanged() { if (enabled) kafkaTemplate.send("customer.changed", body); }
  void publishDynamic(String topic) { kafkaTemplate.send(topic, body); }
}
