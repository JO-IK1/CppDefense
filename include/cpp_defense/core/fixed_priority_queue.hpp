#pragma once

#include <algorithm>
#include <cstddef>
#include <functional>
#include <stdexcept>
#include <utility>
#include <vector>

namespace cpp_defense {

template <typename T, typename Compare = std::greater<T>>
class FixedPriorityQueue {
 public:
  explicit FixedPriorityQueue(std::size_t capacity, Compare compare = Compare{})
      : capacity_(capacity), compare_(std::move(compare)) {
    data_.reserve(capacity_);
  }

  ~FixedPriorityQueue() = default;

  FixedPriorityQueue(const FixedPriorityQueue&) = default;
  FixedPriorityQueue& operator=(const FixedPriorityQueue&) = default;

  FixedPriorityQueue(FixedPriorityQueue&&) = default;
  FixedPriorityQueue& operator=(FixedPriorityQueue&&) = default;

  bool push(const T& value) {
    if (capacity_ == 0) {
      return false;
    }

    if (data_.size() < capacity_) {
      data_.push_back(value);
      std::push_heap(data_.begin(), data_.end(), compare_);
      return true;
    }

    if (!compare_(value, data_.front())) {
      return false;
    }

    std::pop_heap(data_.begin(), data_.end(), compare_);
    data_.back() = value;
    std::push_heap(data_.begin(), data_.end(), compare_);
    return true;
  }

  bool push(T&& value) {
    if (capacity_ == 0) {
      return false;
    }

    if (data_.size() < capacity_) {
      data_.push_back(std::move(value));
      std::push_heap(data_.begin(), data_.end(), compare_);
      return true;
    }

    if (!compare_(value, data_.front())) {
      return false;
    }

    std::pop_heap(data_.begin(), data_.end(), compare_);
    data_.back() = std::move(value);
    std::push_heap(data_.begin(), data_.end(), compare_);
    return true;
  }

  void pop() {
    if (data_.empty()) {
      throw std::out_of_range("FixedPriorityQueue is empty");
    }

    std::pop_heap(data_.begin(), data_.end(), compare_);
    data_.pop_back();
  }

  const T& top() const {
    if (data_.empty()) {
      throw std::out_of_range("FixedPriorityQueue is empty");
    }

    return data_.front();
  }

  const T& operator[](std::size_t index) const {
    return data_[index];
  }

  std::size_t size() const noexcept {
    return data_.size();
  }

  bool empty() const noexcept {
    return data_.empty();
  }

 private:
  std::size_t capacity_;
  Compare compare_;
  std::vector<T> data_;
};

}  // namespace cpp_defense
